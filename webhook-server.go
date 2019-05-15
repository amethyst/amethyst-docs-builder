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
	"github.com/go-chi/hostrouter"
)

var secret string
var scriptPath string

const semverRegex = "^v(0|[1-9]+).(0|[1-9]+)(.(0|[1-9]+))?$"

func main() {
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

	catchAll := chi.NewRouter()
	catchAll.Get("/health", handleHealth)

	trigger := chi.NewRouter()
	trigger.Post("/", handleTrigger)

	r := chi.NewRouter()
	hr := hostrouter.New()

	docsURL := getEnvOr("DOCS_URL", "docs.amethyst.rs")
	bookURL := getEnvOr("BOOK_URL", "book.amethyst.rs")
	triggerURL := getEnvOr("TRIGGER_URL", "hook.amethyst.rs")

	hr.Map(docsURL, serveSubDirectory("docs"))
	hr.Map(bookURL, serveSubDirectory("book"))
	hr.Map(triggerURL, trigger)
	hr.Map("*", catchAll)

	r.Mount("/", hr)

	log.Printf("serving on port %s\n", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), r))
}

func getEnvOr(s, def string) string {
	result := def
	if val, ok := os.LookupEnv(s); ok {
		result = val
	}

	return result
}

func serveSubDirectory(subdir string) chi.Router {
	stablePath := fmt.Sprintf("./public/%s/stable", subdir)
	masterPath := fmt.Sprintf("./public/%s/master", subdir)
	tagsPath := fmt.Sprintf("./public/%s/tags", subdir)

	mustMkDir(stablePath)
	mustMkDir(masterPath)
	mustMkDir(tagsPath)

	stable := http.Dir(stablePath)
	master := http.Dir(masterPath)
	tags := http.Dir(tagsPath)

	stableFs := http.StripPrefix("/stable", http.FileServer(stable))
	masterFs := http.StripPrefix("/master", http.FileServer(master))
	tagsFs := http.FileServer(tags)

	tagsURL := fmt.Sprintf("/{tag:%s}/*", semverRegex)

	r := chi.NewRouter()
	r.Get("/stable", http.RedirectHandler("/stable/", 301).ServeHTTP)
	r.Get("/stable/*", stableFs.ServeHTTP)

	r.Get("/master", http.RedirectHandler("/master/", 301).ServeHTTP)
	r.Get("/master/*", masterFs.ServeHTTP)
	r.Get(tagsURL, tagsFs.ServeHTTP)

	r.Get("/", http.RedirectHandler("/stable/", 301).ServeHTTP)
	return r
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
	go runScript()

	w.WriteHeader(204)
}

func runScript() {
	cmd := exec.Command("/bin/sh", scriptPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("couldn't get stdout: %s\n", err.Error())
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Printf("couldn't get stderr: %s\n", err.Error())
		return
	}

	err = cmd.Start()
	if err != nil {
		log.Printf("couldn't start cmd: %s\n", err.Error())
		return
	}

	stdoutScanner := bufio.NewScanner(stdout)
	stderrScanner := bufio.NewScanner(stderr)

	go print(stdoutScanner, "--->")
	go print(stderrScanner, "!!->")

	log.Print("waiting for output\n")
	err = cmd.Wait()
	if err != nil {
		log.Printf("err waiting for cmd: %s\n", err)
	}
	log.Print("command ran\n")
}

func print(s *bufio.Scanner, pre string) {
	s.Split(bufio.ScanLines)
	for s.Scan() {
		m := s.Text()
		log.Printf("%s %s\n", pre, m)
	}
}
