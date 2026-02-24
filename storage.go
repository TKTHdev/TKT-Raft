package raft

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

type StorageBackend interface {
	SaveState(term int, votedFor int) error
	LoadState() (int, int, error)
	AppendEntry(entry LogEntry) error
	AppendEntries(entries []LogEntry) error
	TruncateLog(index int) error
	LoadLog() ([]LogEntry, error)
	Close() error
}

func NewStorageBackend(id int, async bool, storageType string) (StorageBackend, error) {
	switch storageType {
	case "bitcask":
		return NewBitcaskStorage(id, async)
	case "iouring":
		return NewIoUringStorage(id, async)
	default:
		return NewFileStorage(id, async)
	}
}

type FileStorage struct {
	id         int
	stateFile  *os.File
	logFile    *os.File
	logWriter  *bufio.Writer
	logOffsets []int64
	async      bool
}

func NewFileStorage(id int, async bool) (*FileStorage, error) {
	stateFilename := fmt.Sprintf("raft_state_%d.bin", id)
	logFilename := fmt.Sprintf("raft_log_%d.bin", id)

	sFile, err := os.OpenFile(stateFilename, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}

	lFile, err := os.OpenFile(logFilename, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		sFile.Close()
		return nil, err
	}

	return &FileStorage{
		id:         id,
		stateFile:  sFile,
		logFile:    lFile,
		logWriter:  bufio.NewWriter(lFile),
		logOffsets: []int64{},
		async:      async,
	}, nil
}

func (s *FileStorage) SaveState(term int, votedFor int) error {
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

func (s *FileStorage) LoadState() (int, int, error) {
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

func (s *FileStorage) AppendEntry(entry LogEntry) error {
	offset, err := s.logFile.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	s.logOffsets = append(s.logOffsets, offset)

	if err := binary.Write(s.logWriter, binary.LittleEndian, int64(entry.Term)); err != nil {
		return err
	}
	cmdLen := int64(len(entry.Command))
	if err := binary.Write(s.logWriter, binary.LittleEndian, cmdLen); err != nil {
		return err
	}
	// Write Command
	if _, err := s.logWriter.Write(entry.Command); err != nil {
		return err
	}

	if err := s.logWriter.Flush(); err != nil {
		return err
	}
	if !s.async {
		return s.logFile.Sync()
	}
	return nil
}

func (s *FileStorage) AppendEntries(entries []LogEntry) error {
	if err := s.logWriter.Flush(); err != nil {
		return err
	}
	currentOffset, err := s.logFile.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		s.logOffsets = append(s.logOffsets, currentOffset)

		if err := binary.Write(s.logWriter, binary.LittleEndian, int64(entry.Term)); err != nil {
			return err
		}
		cmdLen := int64(len(entry.Command))
		if err := binary.Write(s.logWriter, binary.LittleEndian, cmdLen); err != nil {
			return err
		}
		if _, err := s.logWriter.Write(entry.Command); err != nil {
			return err
		}

		// Term(8) + CmdLen(8) + Command(len)
		currentOffset += 16 + int64(len(entry.Command))
	}
	if err := s.logWriter.Flush(); err != nil {
		return err
	}
	if !s.async {
		return s.logFile.Sync()
	}
	return nil
}

func (s *FileStorage) TruncateLog(index int) error {
	if index < 0 {
		return nil
	}
	if index >= len(s.logOffsets) {
		return nil
	}

	truncateAt := s.logOffsets[index]

	if err := s.logWriter.Flush(); err != nil {
		return err
	}

	if err := s.logFile.Truncate(truncateAt); err != nil {
		return err
	}

	if _, err := s.logFile.Seek(truncateAt, 0); err != nil {
		return err
	}

	s.logOffsets = s.logOffsets[:index]

	s.logWriter.Reset(s.logFile)

	if !s.async {
		return s.logFile.Sync()
	}
	return nil
}

func (s *FileStorage) LoadLog() ([]LogEntry, error) {
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
		err := binary.Read(reader, binary.LittleEndian, &term)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		offset += 8

		var cmdLen int64
		err = binary.Read(reader, binary.LittleEndian, &cmdLen)
		if err != nil {
			return nil, err
		}
		offset += 8

		cmd := make([]byte, cmdLen)
		if _, err := io.ReadFull(reader, cmd); err != nil {
			return nil, err
		}
		offset += cmdLen

		s.logOffsets = append(s.logOffsets, startOffset)
		logs = append(logs, LogEntry{
			Term:    int(term),
			Command: cmd,
		})
	}

	return logs, nil
}

func (s *FileStorage) Close() error {
	s.logWriter.Flush()
	s.stateFile.Close()
	return s.logFile.Close()
}
