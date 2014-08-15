package main

import (
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/gonuts/go-shlex"
)

type Task struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Playlist string `json:"-"`
	cmd      *exec.Cmd
}

func NewTask(path string) *Task {
	hash := sha256.Sum256([]byte(path))
	t := &Task{
		ID:   fmt.Sprintf("%x", hash)[:7],
		Name: filepath.Base(path),
		Path: path,
	}
	t.Playlist = t.ID + ".m3u8"
	return t
}

func (t *Task) Stop() {
	t.cmd.Process.Signal(syscall.SIGTERM)
}

type Encoder struct {
	m     sync.Mutex
	cmd   []string
	dir   string
	Tasks map[string]*Task
}

func NewEncoder(cmd, dir string) (*Encoder, error) {
	args, err := shlex.Split(cmd)
	if err != nil {
		return nil, err
	}

	e := &Encoder{
		cmd:   args,
		dir:   dir,
		Tasks: make(map[string]*Task),
	}
	return e, nil
}

func (e *Encoder) Encode(t *Task) error {
	e.m.Lock()
	defer e.m.Unlock()
	if _, ok := e.Tasks[t.Name]; ok {
		return nil
	}

	n := len(e.cmd) + 2
	args := make([]string, n)
	copy(args[2:], e.cmd[1:])
	args[0] = "-i"
	args[1] = t.Path
	args[n-1] = t.Playlist

	t.cmd = exec.Command(e.cmd[0], args...)
	t.cmd.Dir = e.dir
	t.cmd.Stdout = os.Stdout
	t.cmd.Stderr = os.Stderr

	if err := t.cmd.Start(); err != nil {
		return err
	}
	e.Tasks[t.ID] = t
	go func() {
		if err := t.cmd.Wait(); err != nil {
			log.Println(err)
			if err := os.Remove(filepath.Join(e.dir, t.Playlist)); err != nil {
				log.Println(err)
			}
		}
		e.m.Lock()
		defer e.m.Unlock()
		delete(e.Tasks, t.ID)
	}()
	return nil
}
