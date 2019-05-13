package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/go-chi/chi"
)

var secret string
var scriptPath string

func main() {
	if _, ok := os.LookupEnv("SECRET"); !ok {
		log.Fatal("secret env variable is not set!")
	}
	secret = os.Getenv("SECRET")

	port := "3000"
	if val, ok := os.LookupEnv("PORT"); ok {
		port = val
	}

	scriptPath = "./run.sh"
	if val, ok := os.LookupEnv("SCRIPT"); ok {
		scriptPath = val
	}

	r := chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("up"))
	})

	r.Post("/trigger", func(w http.ResponseWriter, r *http.Request) {
		digest := r.Header.Get("X-Hub-Signature")
		if digest == "" {
			http.Error(w, "empty secret header", 403)
			return
		}

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		key := []byte(secret)
		h := hmac.New(sha1.New, key)
		h.Write(body)
		str := base64.StdEncoding.EncodeToString(h.Sum(nil))

		var b map[string]interface{}
		if r.Body == nil {
			http.Error(w, "empty body", 400)
			return
		}

		if str != digest {
			log.Printf("sha1 didn't match: %s\n", str)
			http.Error(w, "invalid secret", 403)
			return
		}

		r.Body.Close()
		r.Body = ioutil.NopCloser(bytes.NewBuffer(body))
		err = json.NewDecoder(r.Body).Decode(&b)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		ref, ok := b["ref"].(string)
		if !ok {
			http.Error(w, "no ref present", 400)
			return
		}

		if ref != "refs/heads/master" {
			log.Printf("ignoring push to ref: %s\n", ref)
			w.WriteHeader(204)
			return
		}

		log.Printf("executing script: %s\n", scriptPath)
		go func() {
			cmd := exec.Command("/bin/sh", scriptPath)
			out, err := cmd.Output()
			if err != nil {
				log.Printf("error running script:\n%s\n", err.Error())
				return
			}

			lines := strings.Split(string(out), "\n")
			log.Print("output:\n")

			for _, l := range lines {
				log.Printf("---> %s\n", l)
			}
		}()

		w.WriteHeader(204)
	})

	http.ListenAndServe(fmt.Sprintf(":%s", port), r)
}
