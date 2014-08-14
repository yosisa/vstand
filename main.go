package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/gonuts/go-shlex"
	"github.com/zenazn/goji"
	"github.com/zenazn/goji/web"
)

type Config struct {
	Exts      []string `toml:"extensions"`
	CacheDir  string   `toml:"cache_dir"`
	CacheSize int64    `toml:"cache_size"`
	Encoder   string   `toml:"encoder"`
	Exposes   map[string]string
}

type FileInfo struct {
	Name string `json:"name"`
	Dir  bool   `json:"dir"`
	Size int64  `json:"size"`
}

func main() {
	var config Config
	b, _ := ioutil.ReadFile("config.toml")
	if _, err := toml.Decode(string(b), &config); err != nil {
		panic(err)
	}

	exts := make(map[string]bool)
	for _, ext := range config.Exts {
		exts[ext] = true
	}

	for k, v := range config.Exposes {
		config.Exposes[k] = strings.TrimRight(v, "/")
	}

	config.CacheDir = strings.TrimRight(config.CacheDir, "/")
	if err := os.MkdirAll(config.CacheDir, 0755); err != nil {
		log.Fatal(err)
	}
	cacheDir := NewCacheDir(config.CacheDir, config.CacheSize)
	defer cacheDir.Close()

	api := web.New()
	goji.Handle("/api/*", api)
	api.Use(CORS)
	api.Use(ApplicationJSON)

	api.Get("/api/browse", func(c web.C, w http.ResponseWriter, r *http.Request) {
		var names []string
		for name := range config.Exposes {
			names = append(names, name)
		}
		b, _ := json.Marshal(names)
		w.Write(b)
	})

	api.Get("/api/browse/:name/*", Handler(func(c web.C, w http.ResponseWriter, r *http.Request) error {
		name := c.URLParams["name"]
		root, ok := config.Exposes[name]
		if !ok {
			return errors.New("Not found")
		}

		parts := strings.SplitN(r.URL.Path, "/", 5)
		path := parts[4]
		if path == "" {
			path = "."
		}
		path = root + "/" + path

		fi, err := os.Stat(path)
		if err != nil {
			return errors.New("Not found")
		}
		if !fi.IsDir() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			w.Header().Set("Content-Type", "text/plain")
			io.Copy(w, f)
			return nil
		}

		items := []FileInfo{}
		files, err := ioutil.ReadDir(path)
		if err != nil {
			return err
		}

		for _, fi := range files {
			if fi.IsDir() {
				items = append(items, FileInfo{fi.Name(), fi.IsDir(), fi.Size()})
			} else {
				ext := filepath.Ext(fi.Name())
				if _, ok := exts[ext]; ok {
					items = append(items, FileInfo{fi.Name(), fi.IsDir(), fi.Size()})
				}
			}
		}

		b, _ := json.Marshal(items)
		w.Write(b)
		return nil
	}))

	goji.Get("/video/stream", Handler(func(c web.C, w http.ResponseWriter, r *http.Request) error {
		path := r.URL.Query().Get("path")
		if path == "" {
			return errors.New("Bad request")
		}

		parts := strings.SplitN(path[1:], "/", 2)
		if len(parts) != 2 {
			return errors.New("Bad request")
		}

		root, ok := config.Exposes[parts[0]]
		if !ok {
			return errors.New("Bad request")
		}

		realPath := root + "/" + parts[1]
		hash := sha256.Sum256([]byte(realPath))
		name := fmt.Sprintf("%x", hash)[:7] + ".m3u8"

		b, err := ioutil.ReadFile(config.CacheDir + "/" + name)
		if err == nil {
			w.Header().Set("Content-Type", "application/x-mpegurl")
			w.Write(b)
			return nil
		}

		if !os.IsNotExist(err) {
			return err
		}

		args, err := shlex.Split(config.Encoder)
		if err != nil {
			return err
		}
		cmdName := args[0]
		args = append([]string{"-i", realPath}, args[1:]...)
		args = append(args, name)
		cmd := exec.Command(cmdName, args...)
		cmd.Dir = config.CacheDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Start(); err != nil {
			return err
		}
		go func() {
			if err := cmd.Wait(); err != nil {
				log.Println(err)
			} else {
				log.Printf("Command done: %v", args)
			}
		}()

		for n := 0; n < 30; n++ {
			b, err = ioutil.ReadFile(config.CacheDir + "/" + name)
			if err == nil {
				w.Header().Set("Content-Type", "application/x-mpegurl")
				w.Write(b)
				return nil
			}
			time.Sleep(500 * time.Millisecond)
		}

		return nil
	}))

	goji.Handle("/video/*", http.StripPrefix("/video", http.FileServer(http.Dir(config.CacheDir))))
	goji.Handle("/*", http.FileServer(http.Dir("static")))
	goji.Serve()
}

func Handler(f func(web.C, http.ResponseWriter, *http.Request) error) func(c web.C, w http.ResponseWriter, r *http.Request) {
	return func(c web.C, w http.ResponseWriter, r *http.Request) {
		err := f(c, w, r)
		if err == nil {
			return
		}

		msg := err.Error()
		switch {
		case strings.Contains(msg, "Not found"):
			w.WriteHeader(http.StatusNotFound)
		case strings.Contains(msg, "Bad request"):
			w.WriteHeader(http.StatusBadRequest)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}

		w.Write([]byte(msg))
	}
}
