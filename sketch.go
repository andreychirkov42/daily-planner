package main

import (
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Sketch — рисунок с холста. Сама картинка лежит PNG-файлом в папке sketches рядом с data.json,
// чтобы data.json оставался маленьким.
type Sketch struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

var sketchIDRe = regexp.MustCompile(`^[0-9a-f]{16}$`)

func (s *Store) sketchDir() string { return filepath.Join(filepath.Dir(s.path), "sketches") }

func (s *Store) sketchFile(id string) (string, error) {
	if !sketchIDRe.MatchString(id) {
		return "", ErrNotFound
	}
	return filepath.Join(s.sketchDir(), id+".png"), nil
}

func (s *Store) Sketches() []*Sketch {
	if list := s.Snapshot().Sketches; list != nil {
		return list
	}
	return []*Sketch{} // в JSON — пустой список, а не null
}

// SaveSketch создаёт (id == "") или обновляет рисунок.
func (s *Store) SaveSketch(id, title string, png []byte) (*Sketch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	var sk *Sketch
	if id == "" {
		sk = &Sketch{ID: newID(), CreatedAt: now}
		s.data.Sketches = append([]*Sketch{sk}, s.data.Sketches...)
	} else {
		for _, x := range s.data.Sketches {
			if x.ID == id {
				sk = x
			}
		}
		if sk == nil {
			return nil, ErrNotFound
		}
	}
	if title != "" {
		sk.Title = title
	}
	sk.UpdatedAt = now
	if png != nil {
		path, err := s.sketchFile(sk.ID)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(s.sketchDir(), 0o755); err != nil {
			return nil, err
		}
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, png, 0o644); err != nil {
			return nil, err
		}
		if err := os.Rename(tmp, path); err != nil {
			return nil, err
		}
	}
	cp := *sk
	return &cp, s.saveLocked()
}

func (s *Store) DeleteSketch(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, x := range s.data.Sketches {
		if x.ID == id {
			s.data.Sketches = append(s.data.Sketches[:i], s.data.Sketches[i+1:]...)
			if path, err := s.sketchFile(id); err == nil {
				_ = os.Remove(path)
			}
			return s.saveLocked()
		}
	}
	return ErrNotFound
}

// decodePNG принимает data URL «data:image/png;base64,…» и проверяет, что это PNG.
func decodePNG(dataURL string) ([]byte, error) {
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(dataURL, prefix) {
		return nil, errors.New("ожидается PNG в формате data URL")
	}
	b, err := base64.StdEncoding.DecodeString(dataURL[len(prefix):])
	if err != nil {
		return nil, errors.New("повреждённая картинка")
	}
	if len(b) < 8 || string(b[1:4]) != "PNG" {
		return nil, errors.New("это не PNG")
	}
	return b, nil
}

func registerSketchAPI(mux *http.ServeMux, s *Store) {
	mux.HandleFunc("GET /api/sketches", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.Sketches())
	})
	save := func(w http.ResponseWriter, r *http.Request, id string) {
		var body struct{ Title, PNG string }
		if !readJSON(w, r, &body) {
			return
		}
		var png []byte
		if body.PNG != "" {
			var err error
			if png, err = decodePNG(body.PNG); err != nil {
				httpError(w, http.StatusBadRequest, err.Error())
				return
			}
		}
		sk, err := s.SaveSketch(id, body.Title, png)
		respond(w, http.StatusOK, sk, err)
	}
	mux.HandleFunc("POST /api/sketches", func(w http.ResponseWriter, r *http.Request) { save(w, r, "") })
	mux.HandleFunc("PUT /api/sketches/{id}", func(w http.ResponseWriter, r *http.Request) { save(w, r, r.PathValue("id")) })
	mux.HandleFunc("DELETE /api/sketches/{id}", func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusNoContent, nil, s.DeleteSketch(r.PathValue("id")))
	})
	mux.HandleFunc("GET /api/sketches/{id}/image", func(w http.ResponseWriter, r *http.Request) {
		path, err := s.sketchFile(r.PathValue("id"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, path)
	})
}
