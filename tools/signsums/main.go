// signsums подписывает SHA256SUMS.txt релиза ключом Clepsy (ed25519), чтобы автообновление
// не поставило файл, подменённый на GitHub: без закрытого ключа подпись не подделать.
//
//	go run ./tools/signsums -gen -key D:\Android\clepsy-update.key   создать ключ (один раз), печатает открытый ключ для update.go
//	go run ./tools/signsums -key D:\Android\clepsy-update.key SHA256SUMS.txt   → SHA256SUMS.txt.sig
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

func main() {
	keyPath := flag.String("key", "", "файл закрытого ключа (хранить вне git)")
	gen := flag.Bool("gen", false, "создать новый ключ")
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
	if flag.NArg() != 1 {
		log.Fatal("укажи файл для подписи")
	}
	raw, err := os.ReadFile(*keyPath)
	if err != nil {
		log.Fatal(err)
	}
	seed, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(seed) != ed25519.SeedSize {
		log.Fatal("повреждённый ключ")
	}
	data, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		log.Fatal(err)
	}
	sig := ed25519.Sign(ed25519.NewKeyFromSeed(seed), data)
	if err := os.WriteFile(flag.Arg(0)+".sig", []byte(hex.EncodeToString(sig)+"\n"), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Println("подпись:", flag.Arg(0)+".sig")
}
