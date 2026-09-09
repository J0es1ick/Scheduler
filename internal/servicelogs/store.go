package servicelogs

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var componentPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,80}$`)

type Store struct {
	root        *os.Root
	mu          sync.Mutex
	rotateBytes int64
	writeFailed bool
	slots       chan struct{}
}

func OpenStore(path string) (*Store, error) {
	if err := os.MkdirAll(path, 0700); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	return &Store{root: root, rotateBytes: 10 << 20, slots: make(chan struct{}, 2)}, nil
}
func (s *Store) Close() error { return s.root.Close() }

func (s *Store) Append(component string, at time.Time, message string) (result error) {
	if !componentPattern.MatchString(component) {
		return errors.New("invalid component")
	}
	entry := newEntry(component, at, message)
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	s.mu.Lock()
	defer s.mu.Unlock()
	defer func() { s.writeFailed = result != nil }()
	name := component + ".jsonl"
	if info, err := s.root.Stat(name); err == nil {
		if info.Size()+int64(len(data)) > s.rotateBytes || info.ModTime().UTC().Format(time.DateOnly) != time.Now().UTC().Format(time.DateOnly) {
			if err := s.root.Remove(name + ".4"); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			for n := 3; n >= 0; n-- {
				old := name
				if n > 0 {
					old += "." + strconv.Itoa(n)
				}
				if err := s.root.Rename(old, name+"."+strconv.Itoa(n+1)); err != nil && !errors.Is(err, os.ErrNotExist) {
					return err
				}
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := s.root.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	if info, err := file.Stat(); err == nil && info.Size() > 0 {
		last := []byte{0}
		if _, err = file.ReadAt(last, info.Size()-1); err == nil && last[0] != '\n' {
			if _, err = file.Write([]byte{'\n'}); err != nil {
				return err
			}
		}
	}
	_, err = file.Write(data)
	return err
}

func (s *Store) Purge(now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir, err := s.root.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	files, err := dir.ReadDir(-1)
	if err != nil {
		return err
	}
	for _, file := range files {
		if !logFileName(file.Name()) {
			continue
		}
		info, err := file.Info()
		if err != nil {
			return err
		}
		if info.ModTime().Before(now.Add(-7 * 24 * time.Hour)) {
			if err = s.root.Remove(file.Name()); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}

func logFileName(name string) bool {
	component, suffix, ok := strings.Cut(name, ".jsonl")
	return ok && componentPattern.MatchString(component) && (suffix == "" || suffix == ".1" || suffix == ".2" || suffix == ".3" || suffix == ".4")
}

func (s *Store) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != "GET" || r.URL.Path != "/logs" {
		http.NotFound(w, r)
		return
	}
	q, err := ParseQuery(r.URL.Query(), time.Now())
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		http.Error(w, "Просмотр журналов занят", 429)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	page, err := s.Read(ctx, q)
	if err != nil {
		http.Error(w, "Не удалось прочитать журнал", 503)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(page)
}

func (s *Store) Read(ctx context.Context, q Query) (Page, error) {
	page := Page{Entries: []Entry{}, Components: []Component{}, Modules: []string{}, Warnings: []string{}, CheckedAt: time.Now().UTC()}
	dir, err := s.root.Open(".")
	if err != nil {
		return page, err
	}
	files, err := dir.ReadDir(-1)
	dir.Close()
	if err != nil {
		return page, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })
	components := map[string]bool{}
	modules := map[string]bool{}
	for _, file := range files {
		if logFileName(file.Name()) && file.Type()&os.ModeSymlink == 0 {
			component, _, _ := strings.Cut(file.Name(), ".jsonl")
			components[component] = true
		}
	}
	var scanned int64
	s.mu.Lock()
	failed := s.writeFailed
	s.mu.Unlock()
	if failed {
		page.Warnings = append(page.Warnings, "Сборщик не смог сохранить часть записей. Проверьте свободное место и права на хранилище.")
	}
	for _, file := range files {
		if !logFileName(file.Name()) || file.Type()&os.ModeSymlink != 0 {
			continue
		}
		component, _, _ := strings.Cut(file.Name(), ".jsonl")
		if q.Component != "" && component != q.Component {
			continue
		}
		if ctx.Err() != nil || scanned >= 128<<20 {
			page.Warnings = append(page.Warnings, "Проверена только часть журнала: достигнут лимит чтения. Выберите отдельный компонент.")
			break
		}
		input, err := s.root.Open(file.Name())
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			page.Warnings = append(page.Warnings, "Журнал компонента "+component+" недоступен.")
			continue
		}
		info, err := input.Stat()
		if err != nil {
			input.Close()
			return page, err
		}
		maxBytes := min(info.Size(), (128<<20)-scanned)
		scanner := bufio.NewScanner(io.LimitReader(input, maxBytes))
		scanner.Buffer(make([]byte, 4096), 128<<10)
		invalid := false
		for scanner.Scan() {
			scanned += int64(len(scanner.Bytes()) + 1)
			if ctx.Err() != nil {
				page.Warnings = append(page.Warnings, "Чтение журнала прервано по времени. Выберите отдельный компонент.")
				break
			}
			var entry Entry
			if json.Unmarshal(scanner.Bytes(), &entry) != nil {
				invalid = true
				continue
			}
			if entry.Component != component {
				invalid = true
				continue
			}
			modules[entry.Module] = true
			if entry.Time.Before(q.Since) || entry.Time.After(q.Until) {
				continue
			}
			if q.Before != nil && (entry.Time.After(q.Before.Time) || entry.Time.Equal(q.Before.Time) && entry.ID >= q.Before.ID) {
				continue
			}
			if q.Module != "" && q.Module != entry.Module || q.Source != "" && q.Source != entry.Source || q.Level != "" && q.Level != entry.Level {
				continue
			}
			if q.Search != "" && !strings.Contains(strings.ToLower(string(scanner.Bytes())), strings.ToLower(q.Search)) {
				continue
			}
			page.Entries = append(page.Entries, entry)
			if len(page.Entries) > 2*(q.Limit+1) {
				sortEntries(page.Entries)
				page.Entries = page.Entries[:q.Limit+1]
			}
		}
		if scanner.Err() != nil || invalid {
			page.Warnings = append(page.Warnings, "В журнале "+component+" обнаружены неполные записи.")
		}
		input.Close()
	}
	for component := range components {
		page.Components = append(page.Components, Component{Name: component, State: "журнал"})
	}
	sort.Slice(page.Components, func(i, j int) bool { return page.Components[i].Name < page.Components[j].Name })
	for module := range modules {
		page.Modules = append(page.Modules, module)
	}
	sort.Strings(page.Modules)
	sortEntries(page.Entries)
	if len(page.Entries) > q.Limit {
		page.Entries = page.Entries[:q.Limit]
		last := page.Entries[len(page.Entries)-1]
		page.NextCursor = encodeCursor(Cursor{Time: last.Time, ID: last.ID})
	}
	return page, nil
}

func sortEntries(entries []Entry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Time.Equal(entries[j].Time) {
			return entries[i].ID > entries[j].ID
		}
		return entries[i].Time.After(entries[j].Time)
	})
}

func ParseSyslog(packet []byte, project string) (string, time.Time, string, error) {
	parts := strings.SplitN(strings.TrimSuffix(string(packet), "\n"), " ", 8)
	if len(parts) != 8 || !strings.HasPrefix(parts[0], "<") || !strings.HasSuffix(parts[0], ">1") || parts[6] != "-" {
		return "", time.Time{}, "", errors.New("unsupported syslog format")
	}
	priority, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(parts[0], "<"), ">1"))
	if err != nil || priority < 0 || priority > 191 {
		return "", time.Time{}, "", errors.New("invalid syslog priority")
	}
	at, err := time.Parse(time.RFC3339Nano, parts[1])
	if err != nil {
		return "", at, "", err
	}
	prefix := project + "/"
	if project == "" || !strings.HasPrefix(parts[3], prefix) {
		return "", at, "", errors.New("foreign project")
	}
	component := strings.TrimPrefix(strings.TrimPrefix(parts[3], prefix), "scheduler-")
	if !componentPattern.MatchString(component) {
		return "", at, "", fmt.Errorf("invalid component")
	}
	return component, at, parts[7], nil
}
