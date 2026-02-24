package raft

import (
	"encoding/binary"
	"fmt"
	"io"
	"sort"

	"github.com/octu0/bitcaskdb"
)

type BitcaskStorage struct {
	db       *bitcaskdb.Bitcask
	logCount int
	async    bool
}

func NewBitcaskStorage(id int, async bool) (*BitcaskStorage, error) {
	path := fmt.Sprintf("raft_bitcask_%d", id)
	db, err := bitcaskdb.Open(path)
	if err != nil {
		return nil, err
	}
	return &BitcaskStorage{
		db:    db,
		async: async,
	}, nil
}

var stateKey = []byte("raft:state")

const logKeyPrefix = "raft:log:"

func logKey(index int) []byte {
	return []byte(fmt.Sprintf("raft:log:%010d", index))
}

func (s *BitcaskStorage) SaveState(term int, votedFor int) error {
	buf := make([]byte, 16)
	binary.LittleEndian.PutUint64(buf[0:8], uint64(term))
	binary.LittleEndian.PutUint64(buf[8:16], uint64(votedFor))
	if err := s.db.PutBytes(stateKey, buf); err != nil {
		return err
	}
	if !s.async {
		return s.db.Sync()
	}
	return nil
}

func (s *BitcaskStorage) LoadState() (int, int, error) {
	if !s.db.Has(stateKey) {
		return 0, -2, nil
	}
	rc, err := s.db.Get(stateKey)
	if err != nil {
		return 0, -2, err
	}
	defer rc.Close()
	buf, err := io.ReadAll(rc)
	if err != nil {
		return 0, 0, err
	}
	if len(buf) < 16 {
		return 0, -2, nil
	}
	term := int(binary.LittleEndian.Uint64(buf[0:8]))
	votedFor := int(binary.LittleEndian.Uint64(buf[8:16]))
	return term, votedFor, nil
}

func (s *BitcaskStorage) appendEntryRaw(entry LogEntry) error {
	key := logKey(s.logCount)
	buf := make([]byte, 8+len(entry.Command))
	binary.LittleEndian.PutUint64(buf[0:8], uint64(entry.Term))
	copy(buf[8:], entry.Command)
	if err := s.db.PutBytes(key, buf); err != nil {
		return err
	}
	s.logCount++
	return nil
}

func (s *BitcaskStorage) AppendEntry(entry LogEntry) error {
	if err := s.appendEntryRaw(entry); err != nil {
		return err
	}
	if !s.async {
		return s.db.Sync()
	}
	return nil
}

func (s *BitcaskStorage) AppendEntries(entries []LogEntry) error {
	for _, entry := range entries {
		if err := s.appendEntryRaw(entry); err != nil {
			return err
		}
	}
	if !s.async {
		return s.db.Sync()
	}
	return nil
}

func (s *BitcaskStorage) TruncateLog(index int) error {
	for i := index; i < s.logCount; i++ {
		if err := s.db.Delete(logKey(i)); err != nil {
			return err
		}
	}
	s.logCount = index
	if !s.async {
		return s.db.Sync()
	}
	return nil
}

func (s *BitcaskStorage) LoadLog() ([]LogEntry, error) {
	var keys []string
	err := s.db.Scan([]byte(logKeyPrefix), func(key []byte) error {
		keys = append(keys, string(key))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(keys)

	var logs []LogEntry
	for _, k := range keys {
		rc, err := s.db.Get([]byte(k))
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		if len(data) < 8 {
			return nil, fmt.Errorf("corrupted log entry: %s", k)
		}
		term := int(binary.LittleEndian.Uint64(data[0:8]))
		cmd := data[8:]
		logs = append(logs, LogEntry{Term: term, Command: cmd})
	}
	s.logCount = len(logs)
	return logs, nil
}

func (s *BitcaskStorage) Close() error {
	return s.db.Close()
}
