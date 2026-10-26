// release готовит файлы обновления для релиза Clepsy на GitHub:
// latest.json (версия, что нового, ссылки и SHA-256 сборок), latest.json.sig (подпись ed25519
// ключом автора) и SHA256SUMS.txt (для людей). Автообновление ставит только подписанный манифест.
//
//	go run ./tools/release -gen -key D:\Android\clepsy-update.key        создать ключ (один раз; печатает открытый для update.go)
//	go run ./tools/release -key D:\Android\clepsy-update.key -version 1.3.0 -notes notes.md -exe Clepsy.exe -apk Clepsy.apk -out <папка>
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const repo = "https://github.com/andreychirkov42/daily-planner"

type build struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

func main() {
	keyPath := flag.String("key", "", "файл закрытого ключа (хранить вне git)")
	gen := flag.Bool("gen", false, "создать новый ключ")
	version := flag.String("version", "", "версия релиза, например 1.3.0 (тег будет v1.3.0)")
	notes := flag.String("notes", "", "файл с описанием изменений (Markdown, по-русски)")
	exe := flag.String("exe", "", "Clepsy.exe")
	apk := flag.String("apk", "", "Clepsy.apk")
	out := flag.String("out", ".", "куда положить latest.json, latest.json.sig и SHA256SUMS.txt")
	base := flag.String("base", "", "адрес, где будут лежать файлы (для проверки на своём сервере); по умолчанию — релиз на GitHub")
	flag.Parse()
	if *keyPath == "" {
		log.Fatal("нужен -key")
	}
	if *gen {
		if _, err := os.Stat(*keyPath); err == nil {
			log.Fatalf("%s уже есть — новый ключ сломал бы обновление у всех, кто поставил старые версии", *keyPath)
		}
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(*keyPath, []byte(hex.EncodeToString(priv.Seed())+"\n"), 0o600); err != nil {
			log.Fatal(err)
		}
		fmt.Println("открытый ключ (вставить в updatePubKey в update.go):", hex.EncodeToString(pub))
		return
	}
	if *version == "" || *notes == "" || *exe == "" || *apk == "" {
		log.Fatal("нужны -version, -notes, -exe и -apk")
	}
	raw, err := os.ReadFile(*keyPath)
	if err != nil {
		log.Fatal(err)
	}
	seed, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(seed) != ed25519.SeedSize {
		log.Fatal("повреждённый ключ")
	}
	text, err := os.ReadFile(*notes)
	if err != nil {
		log.Fatal(err)
	}

	m := struct {
		Version   string           `json:"version"`
		PubDate   string           `json:"pub_date"`
		Notes     string           `json:"notes"`
		Platforms map[string]build `json:"platforms"`
	}{
		Version:   strings.TrimPrefix(*version, "v"),
		PubDate:   time.Now().UTC().Format(time.RFC3339),
		Notes:     strings.TrimSpace(strings.ReplaceAll(string(text), "\r\n", "\n")),
		Platforms: map[string]build{},
	}
	if *base == "" {
		*base = fmt.Sprintf("%s/releases/download/v%s", repo, m.Version)
	}
	var sums []string
	for platform, path := range map[string]string{"windows-x86_64": *exe, "android": *apk} {
		b, err := os.ReadFile(path)
		if err != nil {
			log.Fatal(err)
		}
		h := sha256.Sum256(b)
		name := filepath.Base(path)
		m.Platforms[platform] = build{
			URL:    strings.TrimSuffix(*base, "/") + "/" + name,
			SHA256: hex.EncodeToString(h[:]),
			Size:   int64(len(b)),
		}
		sums = append(sums, hex.EncodeToString(h[:])+"  "+name)
	}
	data, _ := json.MarshalIndent(m, "", "  ")
	sig := ed25519.Sign(ed25519.NewKeyFromSeed(seed), data)
	write := func(name string, b []byte) {
		if err := os.WriteFile(filepath.Join(*out, name), b, 0o644); err != nil {
			log.Fatal(err)
		}
	}
	write("latest.json", data)
	write("latest.json.sig", []byte(hex.EncodeToString(sig)+"\n"))
	write("SHA256SUMS.txt", []byte(strings.Join(sums, "\n")+"\n"))
	fmt.Println("готово:", filepath.Join(*out, "latest.json"), "+ .sig + SHA256SUMS.txt")
}
