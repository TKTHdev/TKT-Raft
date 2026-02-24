package raft

// IoUringStorage is a StorageBackend that uses io_uring for log writes.
//
// Architecture:
//
//   AppendEntry / AppendEntries
//       │  send writeJob to writeCh
//       ↓
//   submitter goroutine (owns ioRing)
//       │  drain all pending writeJobs (group commit)
//       │  concatenate data → one Pwrite + one Fdatasync via io_uring
//       ↓
//   notify each caller with its starting file offset
//
// Group commit happens naturally: while one Pwrite+Fdatasync is in flight
// in io_uring_enter, new jobs accumulate in writeCh.  On the next loop
// iteration the submitter drains all of them and issues a single fsync
// covering every entry.

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sync"
)

// writeJob is sent by AppendEntry / AppendEntries to the submitter goroutine.
type writeJob struct {
	data     []byte      // pre-encoded log bytes (one or more entries)
	resultCh chan writeResult
}

// writeResult is returned by the submitter to the caller.
type writeResult struct {
	startOffset int64 // file offset where this job's data begins
	err         error
}

// truncateJob is sent by TruncateLog to the submitter goroutine.
type truncateJob struct {
	at     int64
	doneCh chan error
}

// IoUringStorage implements StorageBackend using io_uring.
type IoUringStorage struct {
	stateFile  *os.File
	logFile    *os.File
	logFd      int
	logOffsets []int64 // maintained by AppendEntry/AppendEntries callers
	async      bool

	ring       *ioRing
	writeCh    chan writeJob
	truncateCh chan truncateJob
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

func NewIoUringStorage(id int, async bool) (*IoUringStorage, error) {
	stateFilename := fmt.Sprintf("raft_state_%d.bin", id)
	logFilename := fmt.Sprintf("raft_log_%d.bin", id)

	sFile, err := os.OpenFile(stateFilename, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}

	// O_RDWR|O_CREAT for log; O_DIRECT is omitted to keep aligned-write
	// complexity out of scope.
	lFile, err := os.OpenFile(logFilename, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		sFile.Close()
		return nil, err
	}

	info, err := lFile.Stat()
	if err != nil {
		sFile.Close()
		lFile.Close()
		return nil, err
	}

	ring, err := newIoRing(16)
	if err != nil {
		sFile.Close()
		lFile.Close()
		return nil, fmt.Errorf("io_uring unavailable: %w", err)
	}

	s := &IoUringStorage{
		stateFile:  sFile,
		logFile:    lFile,
		logFd:      int(lFile.Fd()),
		logOffsets: []int64{},
		async:      async,
		ring:       ring,
		writeCh:    make(chan writeJob, 256),
		truncateCh: make(chan truncateJob, 1),
		stopCh:     make(chan struct{}),
	}

	s.wg.Add(1)
	go s.runSubmitter(info.Size())
	return s, nil
}

// runSubmitter is the single goroutine that owns the ioRing.
func (s *IoUringStorage) runSubmitter(initialOffset int64) {
	defer s.wg.Done()
	currentOffset := initialOffset

	for {
		// Wait for the first event.
		var jobs []writeJob
		select {
		case job := <-s.writeCh:
			jobs = append(jobs, job)
		case trunc := <-s.truncateCh:
			err := s.doTruncate(trunc.at)
			if err == nil {
				currentOffset = trunc.at
			}
			trunc.doneCh <- err
			continue
		case <-s.stopCh:
			return
		}

		// Drain any additional write jobs that are already queued
		// (group commit: batch them into one io_uring submission).
	drain:
		for {
			select {
			case job := <-s.writeCh:
				jobs = append(jobs, job)
			default:
				break drain
			}
		}

		// Calculate per-job starting offsets and concatenate all data.
		offsets := make([]int64, len(jobs))
		off := currentOffset
		var totalSize int
		for _, j := range jobs {
			totalSize += len(j.data)
		}
		allData := make([]byte, 0, totalSize)
		for i, j := range jobs {
			offsets[i] = off
			allData = append(allData, j.data...)
			off += int64(len(j.data))
		}

		// Submit to io_uring.
		var err error
		if s.async {
			err = s.ring.submitWriteAsync(s.logFd, allData, currentOffset)
		} else {
			err = s.ring.submitWriteSync(s.logFd, allData, currentOffset)
		}

		if err == nil {
			currentOffset = off
		}

		// Notify all callers with their individual starting offsets.
		for i, j := range jobs {
			startOff := offsets[i]
			if err != nil {
				startOff = -1
			}
			j.resultCh <- writeResult{startOffset: startOff, err: err}
		}
	}
}

func (s *IoUringStorage) doTruncate(at int64) error {
	if err := s.logFile.Truncate(at); err != nil {
		return err
	}
	if !s.async {
		return s.logFile.Sync()
	}
	return nil
}

// SaveState uses regular file I/O (state writes are infrequent).
func (s *IoUringStorage) SaveState(term int, votedFor int) error {
	if _, err := s.stateFile.Seek(0, 0); err != nil {
		return err
	}
	buf := make([]byte, 16)
	binary.LittleEndian.PutUint64(buf[0:8], uint64(term))
	binary.LittleEndian.PutUint64(buf[8:16], uint64(votedFor))
	if _, err := s.stateFile.Write(buf); err != nil {
		return err
	}
	if !s.async {
		return s.stateFile.Sync()
	}
	return nil
}

func (s *IoUringStorage) LoadState() (int, int, error) {
	info, err := s.stateFile.Stat()
	if err != nil {
		return 0, -2, err
	}
	if info.Size() == 0 {
		return 0, -2, nil
	}
	if _, err := s.stateFile.Seek(0, 0); err != nil {
		return 0, 0, err
	}
	buf := make([]byte, 16)
	if _, err := io.ReadFull(s.stateFile, buf); err != nil {
		return 0, 0, err
	}
	term := int(binary.LittleEndian.Uint64(buf[0:8]))
	votedFor := int(binary.LittleEndian.Uint64(buf[8:16]))
	return term, votedFor, nil
}

// encodeLogEntry serialises a LogEntry to the same binary format as
// FileStorage: term (int64 LE) + cmdLen (int64 LE) + command bytes.
func encodeLogEntry(entry LogEntry) []byte {
	buf := make([]byte, 16+len(entry.Command))
	binary.LittleEndian.PutUint64(buf[0:8], uint64(entry.Term))
	binary.LittleEndian.PutUint64(buf[8:16], uint64(len(entry.Command)))
	copy(buf[16:], entry.Command)
	return buf
}

func (s *IoUringStorage) AppendEntry(entry LogEntry) error {
	data := encodeLogEntry(entry)
	resultCh := make(chan writeResult, 1)
	s.writeCh <- writeJob{data: data, resultCh: resultCh}
	res := <-resultCh
	if res.err != nil {
		return res.err
	}
	s.logOffsets = append(s.logOffsets, res.startOffset)
	return nil
}

func (s *IoUringStorage) AppendEntries(entries []LogEntry) error {
	if len(entries) == 0 {
		return nil
	}

	// Encode all entries into one contiguous buffer so the submitter can
	// write them in a single Pwrite.
	entrySizes := make([]int, len(entries))
	var allData []byte
	for i, e := range entries {
		enc := encodeLogEntry(e)
		entrySizes[i] = len(enc)
		allData = append(allData, enc...)
	}

	resultCh := make(chan writeResult, 1)
	s.writeCh <- writeJob{data: allData, resultCh: resultCh}
	res := <-resultCh
	if res.err != nil {
		return res.err
	}

	// Track the starting offset of each individual entry.
	off := res.startOffset
	for i := range entries {
		s.logOffsets = append(s.logOffsets, off)
		off += int64(entrySizes[i])
	}
	return nil
}

func (s *IoUringStorage) TruncateLog(index int) error {
	if index < 0 {
		return nil
	}
	if index >= len(s.logOffsets) {
		return nil
	}
	truncateAt := s.logOffsets[index]
	doneCh := make(chan error, 1)
	s.truncateCh <- truncateJob{at: truncateAt, doneCh: doneCh}
	if err := <-doneCh; err != nil {
		return err
	}
	s.logOffsets = s.logOffsets[:index]
	return nil
}

// LoadLog reads the log file sequentially using buffered I/O.
// Called only at startup, before any writes begin.
func (s *IoUringStorage) LoadLog() ([]LogEntry, error) {
	if _, err := s.logFile.Seek(0, 0); err != nil {
		return nil, err
	}

	var logs []LogEntry
	s.logOffsets = []int64{}

	reader := bufio.NewReader(s.logFile)
	offset := int64(0)

	for {
		startOffset := offset

		var term int64
		if err := binary.Read(reader, binary.LittleEndian, &term); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		offset += 8

		var cmdLen int64
		if err := binary.Read(reader, binary.LittleEndian, &cmdLen); err != nil {
			return nil, err
		}
		offset += 8

		cmd := make([]byte, cmdLen)
		if _, err := io.ReadFull(reader, cmd); err != nil {
			return nil, err
		}
		offset += cmdLen

		s.logOffsets = append(s.logOffsets, startOffset)
		logs = append(logs, LogEntry{Term: int(term), Command: cmd})
	}

	return logs, nil
}

func (s *IoUringStorage) Close() error {
	close(s.stopCh)
	s.wg.Wait()
	s.ring.close()
	s.stateFile.Close()
	return s.logFile.Close()
}
