package store

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/relentlessworks/flagkit/internal/model"
)

// Store is a JSON file-backed data store.
type Store struct {
	mu       sync.RWMutex
	filePath string
	data     *storeData
}

type storeData struct {
	Workspaces map[string]*model.Workspace `json:"workspaces"`
	Tokens     map[string]*model.Token     `json:"tokens"`
	OTPs       map[string]*model.OTP       `json:"otps"`
	Flags      map[string]map[string]*model.Flag `json:"flags"` // ws_handle -> flag_handle -> flag
	AuditLog   []model.AuditEntry         `json:"audit_log"`
	auditSeq   int                         `json:"-"`
}

// New creates a new store backed by the given file path.
func New(filePath string) (*Store, error) {
	s := &Store{
		filePath: filePath,
		data: &storeData{
			Workspaces: make(map[string]*model.Workspace),
			Tokens:     make(map[string]*model.Token),
			OTPs:       make(map[string]*model.OTP),
			Flags:      make(map[string]map[string]*model.Flag),
			AuditLog:   []model.AuditEntry{},
		},
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// load reads the JSON file from disk. If the file doesn't exist, it's a fresh start.
func (s *Store) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read store file: %w", err)
	}
	if len(data) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := json.Unmarshal(data, s.data); err != nil {
		return fmt.Errorf("unmarshal store: %w", err)
	}
	if s.data.Workspaces == nil {
		s.data.Workspaces = make(map[string]*model.Workspace)
	}
	if s.data.Tokens == nil {
		s.data.Tokens = make(map[string]*model.Token)
	}
	if s.data.OTPs == nil {
		s.data.OTPs = make(map[string]*model.OTP)
	}
	if s.data.Flags == nil {
		s.data.Flags = make(map[string]map[string]*model.Flag)
	}
	if s.data.AuditLog == nil {
		s.data.AuditLog = []model.AuditEntry{}
	}
	// Restore audit sequence
	s.data.auditSeq = len(s.data.AuditLog)
	return nil
}

// save writes the store data to disk.
func (s *Store) save() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal store: %w", err)
	}
	if err := os.WriteFile(s.filePath, data, 0644); err != nil {
		return fmt.Errorf("write store file: %w", err)
	}
	return nil
}

// --- Workspace ---

func (s *Store) CreateWorkspace(ws *model.Workspace) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.data.Workspaces[ws.Handle]; exists {
		return fmt.Errorf("workspace already exists")
	}
	s.data.Workspaces[ws.Handle] = ws
	s.data.Flags[ws.Handle] = make(map[string]*model.Flag)
	return s.save()
}

func (s *Store) GetWorkspace(handle string) (*model.Workspace, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ws, ok := s.data.Workspaces[handle]
	if !ok {
		return nil, fmt.Errorf("workspace not found")
	}
	return ws, nil
}

// --- Token ---

func (s *Store) SaveToken(t *model.Token) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Tokens[t.Token] = t
	return s.save()
}

func (s *Store) GetToken(token string) (*model.Token, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.data.Tokens[token]
	if !ok {
		return nil, fmt.Errorf("token not found")
	}
	return t, nil
}

// --- OTP ---

func (s *Store) SaveOTP(otp *model.OTP) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.OTPs[otp.Email] = otp
	return s.save()
}

func (s *Store) GetOTP(email string) (*model.OTP, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	otp, ok := s.data.OTPs[email]
	if !ok {
		return nil, fmt.Errorf("no OTP for email")
	}
	return otp, nil
}

func (s *Store) DeleteOTP(email string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data.OTPs, email)
	_ = s.save()
}

// --- Flags ---

func (s *Store) CreateFlag(wsHandle string, f *model.Flag) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	flags, ok := s.data.Flags[wsHandle]
	if !ok {
		return fmt.Errorf("workspace not found")
	}
	if _, exists := flags[f.Handle]; exists {
		return fmt.Errorf("flag already exists")
	}
	flags[f.Handle] = f
	return s.save()
}

func (s *Store) GetFlag(wsHandle, handle string) (*model.Flag, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	flags, ok := s.data.Flags[wsHandle]
	if !ok {
		return nil, fmt.Errorf("workspace not found")
	}
	f, ok := flags[handle]
	if !ok {
		return nil, fmt.Errorf("flag not found")
	}
	return f, nil
}

func (s *Store) ListFlags(wsHandle string) ([]*model.Flag, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	flags, ok := s.data.Flags[wsHandle]
	if !ok {
		return nil, fmt.Errorf("workspace not found")
	}
	result := make([]*model.Flag, 0, len(flags))
	for _, f := range flags {
		result = append(result, f)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, nil
}

func (s *Store) UpdateFlag(wsHandle string, f *model.Flag) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	flags, ok := s.data.Flags[wsHandle]
	if !ok {
		return fmt.Errorf("workspace not found")
	}
	if _, exists := flags[f.Handle]; !exists {
		return fmt.Errorf("flag not found")
	}
	f.UpdatedAt = time.Now()
	flags[f.Handle] = f
	return s.save()
}

func (s *Store) DeleteFlag(wsHandle, handle string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	flags, ok := s.data.Flags[wsHandle]
	if !ok {
		return fmt.Errorf("workspace not found")
	}
	if _, exists := flags[handle]; !exists {
		return fmt.Errorf("flag not found")
	}
	delete(flags, handle)
	return s.save()
}

// --- Audit Log ---

func (s *Store) AddAudit(wsHandle string, entry model.AuditEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.auditSeq++
	entry.ID = s.data.auditSeq
	entry.Timestamp = time.Now()
	s.data.AuditLog = append(s.data.AuditLog, entry)
	// Keep last 500 entries
	if len(s.data.AuditLog) > 500 {
		s.data.AuditLog = s.data.AuditLog[len(s.data.AuditLog)-500:]
	}
	return s.save()
}

func (s *Store) ListAudit(wsHandle string, limit int) ([]model.AuditEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	// Filter by workspace handle in the entry
	var result []model.AuditEntry
	for _, e := range s.data.AuditLog {
		result = append(result, e)
	}
	// Return last N entries in reverse order
	if len(result) > limit {
		result = result[len(result)-limit:]
	}
	// Reverse
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result, nil
}

// Data returns a read-only view of the store's internal data.
// This is used by the auth module for workspace lookups.
func (s *Store) Data() *storeData {
	return s.data
}
