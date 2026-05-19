package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/finfreezer/Chirpy/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type RespType string

type RespJson struct {
	Type   RespType
	Data   *json.RawMessage
	Status int
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

type apiConfig struct {
	fileserverHits atomic.Int32
	database       *database.Queries
	platform       string
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

type ChirpBody struct {
	Body string `json:"body"`
}

type UserParameters struct {
	Email string `json:"email"`
}

type returnError struct {
	Error string `json:"error"`
}

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	platform := os.Getenv("PLATFORM")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	dbQueries := database.New(db)
	a := &apiConfig{atomic.Int32{}, dbQueries, platform}
	newMux := http.NewServeMux()
	newMux.Handle("/app/", a.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))
	newMux.HandleFunc("GET /api/healthz", handlerReadiness)
	newMux.HandleFunc("GET /admin/metrics", a.handlerWriteMetrics)
	newMux.HandleFunc("POST /admin/reset", a.handlerResetMetrics)
	//newMux.HandleFunc("POST /api/validate_chirp", a.handlerValidateChirp)
	newMux.HandleFunc("POST /api/users", a.handlerCreateUser)
	newMux.HandleFunc("POST /api/chirps", a.handlerChirp)
	newServer := http.Server{Addr: ":8080", Handler: newMux}
	err = newServer.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}

func (a *apiConfig) handlerChirp(w http.ResponseWriter, r *http.Request) {
	resp := decoder(r, "PostChirp")
	if resp.Status == 500 || resp.Status == 400 {
		return
	}
	helperPostChirp(w, &resp)
	if resp.Status == 500 {
		log.Printf("Something went wrong.")
		return
	}
}

func (a *apiConfig) handlerCreateUser(w http.ResponseWriter, r *http.Request) {

	resp := decoder(r, "CreateUser")
	userInfo := UserParameters{}
	err := json.Unmarshal(*resp.Data, &userInfo)
	newUser, err := a.database.CreateUser(context.Background(), userInfo.Email)
	if err != nil {
		log.Fatal(err)
	}
	responseUser := User{ID: newUser.ID, CreatedAt: newUser.CreatedAt,
		UpdatedAt: newUser.UpdatedAt, Email: newUser.Email}
	if resp.Status == 500 {
		log.Printf("Something went wrong decoding the email.")
		return
	}

	dat, err := json.Marshal(responseUser)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	w.Write(dat)
}

func handlerReadiness(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", " text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(http.StatusText(http.StatusOK)))
}

/*
	func (a *apiConfig) handlerValidateChirp(w http.ResponseWriter, r *http.Request) {
		header, textBody := decoder(w, r)
		if header == 500 {
			return
		}
		header = encoder(w, r, header, textBody)
		if header == 500 {
			log.Printf("Something went wrong.")
			return
		}
	}
*/
func (a *apiConfig) handlerWriteMetrics(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	template := `
<html>
  <body>
    <h1>Welcome, Chirpy Admin</h1>
    <p>Chirpy has been visited %d times!</p>
  </body>
</html>`
	//hits := strconv.Itoa(int(a.fileserverHits.Load()))
	//hitString := fmt.Sprintf("Hits: %d", a.fileserverHits.Load())
	hitString := fmt.Sprintf(template, a.fileserverHits.Load())
	w.Header().Add("Content-Type", " text/html; charset=utf-8")
	w.Write([]byte(hitString))
}

func (a *apiConfig) handlerResetMetrics(w http.ResponseWriter, r *http.Request) {
	if a.platform != "dev" {
		w.WriteHeader(403)
		forbiddenString := fmt.Sprintln("403 Access forbidden. For admin use only.")
		w.Write([]byte(forbiddenString))
		return
	}
	a.database.DeleteUsers(context.Background())
	w.WriteHeader(http.StatusOK)
	old := a.fileserverHits.Swap(0)
	fmt.Fprintf(w, "Hits reset from %d to 0.\n", old)
}

func (a *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

// Possible requests: PostChirp, CreateUser
func decoder(r *http.Request, Type string) RespJson {

	resp := RespJson{}
	var err error = nil
	decoder := json.NewDecoder(r.Body)
	err = decoder.Decode(&resp.Data)

	switch Type {

	case "CreateUser":
		if err != nil {
			log.Printf("Error: %s\n", err)
			resp.Status = 500
			return resp
		}
		resp.Status = 200
		return resp

	case "PostChirp":
		err = decoder.Decode(&resp.Data)
		chirpText := ChirpBody{}
		json.Unmarshal(*resp.Data, &chirpText)
		if err != nil {
			log.Printf("Error: %s\n", err)
			resp.Status = 500
			return resp
		}

		if len(chirpText.Body) > 140 {
			log.Printf("Body too large.")
			resp.Status = 400
			return resp
		}
		return resp
	}

	resp.Status = 500
	return resp
}

func searchProfanity(text string) string {
	badWords := []string{"kerfuffle", "sharbert", "fornax"}
	newText := strings.Split(text, " ")

	for i, originalWord := range newText {
		for _, badWord := range badWords {
			if strings.ToLower(originalWord) == badWord {
				newText[i] = "****"
			}
		}
	}

	words := strings.Join(newText, " ")

	return words
}

func helperPostChirp(w http.ResponseWriter, data *RespJson) {

	if data.Status == 400 {
		respBody := returnError{Error: "Chirp is too long"}
		dat, err := json.Marshal(respBody)
		if err != nil {
			log.Printf("Error marshalling JSON: %s", err)
			w.WriteHeader(500)
			data.Status = 500
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		w.Write(dat)
		data.Status = 400
		return
	}

	if data.Status == 200 {
		body := ChirpBody{}
		err := json.Unmarshal(*data.Data, &body)
		if err != nil {
			log.Printf("Error unmarshalling JSON in helperPostChirp: %s", err)
		}
		cleanedResponse := searchProfanity(body.Body)
		if data.Status == 500 {
			log.Printf("Error decoding JSON")
			return
		}
		chirp := Chirp{}
		json.Unmarshal(*data.Data, &chirp)
		chirp.Body = cleanedResponse
		dat, err := json.Marshal(&chirp)
		if err != nil {
			log.Printf("Error marshalling JSON: %s", err)
			w.WriteHeader(500)
			data.Status = 500
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write(dat)
		data.Status = 200
		return
	}
	data.Status = 500
}
