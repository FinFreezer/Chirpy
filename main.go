package main

import (
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
)

type apiConfig struct {
	fileserverHits atomic.Int32
}

func main() {
	a := &apiConfig{atomic.Int32{}}
	newMux := http.NewServeMux()
	newMux.Handle("/app/", a.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))
	newMux.HandleFunc("GET /healthz", handlerReadiness)
	newMux.HandleFunc("GET /metrics", a.handlerWriteMetrics)
	newMux.HandleFunc("POST /reset", a.handlerResetMetrics)
	newServer := http.Server{Addr: ":8080", Handler: newMux}
	err := newServer.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}

func handlerReadiness(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", " text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(http.StatusText(http.StatusOK)))
}

func (a *apiConfig) handlerWriteMetrics(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	//hits := strconv.Itoa(int(a.fileserverHits.Load()))
	hitString := fmt.Sprintf("Hits: %d", a.fileserverHits.Load())
	w.Write([]byte(hitString))
}

func (a *apiConfig) handlerResetMetrics(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	old := a.fileserverHits.Swap(0)
	fmt.Fprintf(w, "Hits reset from %d to 0.", old)
}

func (a *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}
