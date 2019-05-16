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
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/hostrouter"
	"github.com/go-chi/render"
)

var secret string
var scriptPath string
var notFoundPage string

const semverRegex = "^v(0|[1-9]+).(0|[1-9]+)(.(0|[1-9]+))?$"

func main() {
	f, err := ioutil.ReadFile("./404.html")
	if err != nil {
		log.Fatalf("couldn't read 404 page: %s\n", err.Error())
	}

	notFoundPage = string(f)

	secret = getEnvOr("SECRET", "123")
	port := getEnvOr("PORT", "3000")
	scriptPath = getEnvOr("SCRIPT", "./run.sh")

	log.Printf("using script %s\n", scriptPath)

	catchAll := chi.NewRouter()
	catchAll.Get("/health", handleHealth)
	catchAll.NotFound(handleNotFound)

	trigger := chi.NewRouter()
	trigger.Post("/", handleTrigger)
	trigger.NotFound(handleNotFound)

	r := chi.NewRouter()
	hr := hostrouter.New()

	docsURL := getEnvOr("DOCS_URL", "docs.amethyst.rs")
	bookURL := getEnvOr("BOOK_URL", "book.amethyst.rs")
	triggerURL := getEnvOr("TRIGGER_URL", "hook.amethyst.rs")

	docsBaseURL := getEnvOr("DOCS_BASE_URL", "docs.amethyst.rs")
	bookBaseURL := getEnvOr("BOOK_BASE_URL", "book.amethyst.rs")

	log.Printf("using docs base url: %s\n", docsBaseURL)
	log.Printf("using book base url: %s\n", bookBaseURL)

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	hr.Map(docsURL, serveSubDirectory("docs", "/amethyst/", docsBaseURL))
	hr.Map(bookURL, serveSubDirectory("book", "/", bookBaseURL))
	hr.Map(triggerURL, trigger)
	hr.Map("*", catchAll)

	r.Mount("/", hr)
	r.NotFound(handleNotFound)

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

func handleNotFound(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	render.HTML(w, r, notFoundPage)
}

func serveSubDirectory(subdir, root, baseURL string) chi.Router {
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
	stableRoot := fmt.Sprintf("//%s/stable%s", baseURL, root)
	r.Get("/stable", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, stableRoot, 301)
	})
	r.Get("/stable/*", stableFs.ServeHTTP)

	masterRoot := fmt.Sprintf("//%s/master%s", baseURL, root)
	r.Get("/master", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, masterRoot, 301)
	})
	r.Get("/master/*", masterFs.ServeHTTP)
	r.Get(tagsURL, tagsFs.ServeHTTP)

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, stableRoot, 301)
	})
	r.NotFound(handleNotFound)

	return r
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
