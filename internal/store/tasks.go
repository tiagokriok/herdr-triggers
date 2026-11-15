package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/tiagokriok/herdr-triggers/internal/task"
)

func LoadTasks(file string) (task.File, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return task.File{Tasks: []task.Task{}}, nil
	}
	if err != nil {
		return task.File{}, err
	}
	return task.Parse(data)
}

func WriteJSON(file string, v any) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(file), filepath.Base(file)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), file)
}

func withLock(file string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	lock, err := os.OpenFile(file+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		os.Remove(lock.Name())
		lock.Close()
	}()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	return fn()
}

type rawFile struct {
	Tasks []map[string]any `json:"tasks"`
}

func editTasks(file string, edit func(*rawFile) error) error {
	return withLock(file, func() error {
		raw := rawFile{Tasks: []map[string]any{}}
		data, err := os.ReadFile(file)
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			return err
		default:
			if err := json.Unmarshal(data, &raw); err != nil {
				return fmt.Errorf("invalid json: %w", err)
			}
		}
		if err := edit(&raw); err != nil {
			return err
		}
		out, _ := json.Marshal(raw)
		if _, err := task.Parse(out); err != nil {
			return err
		}
		return WriteJSON(file, raw)
	})
}

func indexOf(raw *rawFile, id string) int {
	for i, t := range raw.Tasks {
		if t["id"] == id {
			return i
		}
	}
	return -1
}

func SetEnabled(file, id string, enabled bool) error {
	return editTasks(file, func(raw *rawFile) error {
		i := indexOf(raw, id)
		if i < 0 {
			return fmt.Errorf("unknown task: %s", id)
		}
		raw.Tasks[i]["enabled"] = enabled
		return nil
	})
}

func Disable(file string, ids ...string) error {
	return editTasks(file, func(raw *rawFile) error {
		for _, id := range ids {
			if i := indexOf(raw, id); i >= 0 {
				raw.Tasks[i]["enabled"] = false
			}
		}
		return nil
	})
}

func SaveTask(file string, t map[string]any) error {
	return editTasks(file, func(raw *rawFile) error {
		id, _ := t["id"].(string)
		if i := indexOf(raw, id); i >= 0 {
			raw.Tasks[i] = t
		} else {
			raw.Tasks = append(raw.Tasks, t)
		}
		return nil
	})
}

func DeleteTask(file, id string) error {
	return editTasks(file, func(raw *rawFile) error {
		i := indexOf(raw, id)
		if i < 0 {
			return fmt.Errorf("unknown task: %s", id)
		}
		raw.Tasks = append(raw.Tasks[:i], raw.Tasks[i+1:]...)
		return nil
	})
}
