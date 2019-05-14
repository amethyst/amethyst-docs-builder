package main

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"

	"github.com/go-chi/chi"
)

var secret string
var scriptPath string

const semverRegex = "^v(0|[1-9]+).(0|[1-9]+)(.(0|[1-9]+))?$"

func main() {
	stablePath := "./public/stable"
	masterPath := "./public/master"
	tagsPath := "./public/tags"

	mustMkDir(stablePath)
	mustMkDir(masterPath)
	mustMkDir(tagsPath)

	stable := http.Dir(stablePath)
	master := http.Dir(masterPath)
	tags := http.Dir(tagsPath)

	stableFs := http.StripPrefix(stablePath, http.FileServer(stable))
	masterFs := http.StripPrefix(masterPath, http.FileServer(master))
	tagsFs := http.StripPrefix(tagsPath, http.FileServer(tags))

	if _, ok := os.LookupEnv("SECRET"); !ok {
		log.Fatal("secret env variable is not set!\n")
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

	tagsURL := fmt.Sprintf("/{tag:%s}/*", semverRegex)

	r := chi.NewRouter()
	r.Get("/health", handleHealth)
	r.Post("/trigger", handleTrigger)

	r.Get("/stable/*", stableFs.ServeHTTP)
	r.Get("/master/*", masterFs.ServeHTTP)
	r.Get(tagsURL, tagsFs.ServeHTTP)

	r.Get("/*", redirectToStable)

	log.Printf("serving on port %s\n", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), r))
}

func redirectToStable(w http.ResponseWriter, r *http.Request) {
	p := "/stable" + r.URL.Path
	http.Redirect(w, r, p, 301)
}

func mustMkDir(p string) {
	if err := os.MkdirAll(p, 0755); err != nil {
		log.Fatalf("could not create dir %s: %s\n", p, err.Error())
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
	w.Write([]byte("up"))
}

func handleTrigger(w http.ResponseWriter, r *http.Request) {
	eventType := r.Header.Get("X-GitHub-Event")
	if eventType != "push" {
		log.Printf("ignoring non-push event: %s\n", eventType)
		w.WriteHeader(204)
		return
	}

	if r.Body == nil {
		http.Error(w, "empty body", 400)
		return
	}

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
	hex := hex.EncodeToString(h.Sum(nil))
	calc := fmt.Sprintf("sha1=%s", hex)

	if calc != digest {
		log.Printf("sha1 didn't match: %s\n", calc)
		http.Error(w, "invalid secret", 403)
		return
	}

	r.Body.Close()
	r.Body = ioutil.NopCloser(bytes.NewBuffer(body))

	var b map[string]interface{}
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
	}()

	w.WriteHeader(204)
}

func runScript() {
	cmd := exec.Command("/bin/sh", scriptPath)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	cmd.Start()

	stdoutScanner := bufio.NewScanner(stdout)
	stderrScanner := bufio.NewScanner(stderr)

	stdoutScanner.Split(bufio.ScanLines)
	for stdoutScanner.Scan() {
		m := stdoutScanner.Text()
		log.Printf("---> %s\n", m)
	}

	// flush stderr
	stderrScanner.Split(bufio.ScanLines)
	for stderrScanner.Scan() {
		m := stderrScanner.Text()
		log.Printf("!--> %s\n", m)
	}

	cmd.Wait()
}
