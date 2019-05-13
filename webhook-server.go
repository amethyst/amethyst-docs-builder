package main

import (
	"encoding/json"
	"fmt"
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
		var b map[string]interface{}
		if r.Body == nil {
			http.Error(w, "empty body", 400)
			return
		}

		err := json.NewDecoder(r.Body).Decode(&b)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		config, ok := b["config"].(map[string]interface{})
		if !ok {
			http.Error(w, "config map not present", 400)
			return
		}

		s, ok := config["secret"].(string)
		if !ok {
			http.Error(w, "no secret provided", 400)
			return
		}

		if s != secret {
			http.Error(w, "invalid secret", 403)
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
