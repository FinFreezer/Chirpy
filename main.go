package main

import (
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
	"encoding/json"
	"strings"
	_ "github.com/lib/pq"
	"database/sql"
	"os"
	"time"
	"github.com/joho/godotenv"
	"github.com/finfreezer/Chirpy/internal/database"
	"github.com/google/uuid"
	"context"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	database *database.Queries
	platform string
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
	newMux.HandleFunc("POST /api/validate_chirp", a.handlerValidateChirp)
	newMux.HandleFunc("POST /api/users", a.handlerCreateUser)
	newServer := http.Server{Addr: ":8080", Handler: newMux}
	err = newServer.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}

func (a *apiConfig) handlerCreateUser(w http.ResponseWriter, r *http.Request) {
	type User struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Email     string    `json:"email"`
	}

	header, email := decoder(w, r)
	newUser, err := a.database.CreateUser(context.Background(), email)
	if err != nil {
		log.Fatal(err)
	}
	responseUser := User{ID: newUser.ID, CreatedAt: newUser.CreatedAt, UpdatedAt: newUser.UpdatedAt, Email: email}
	if header == 500 {
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
	return
}

func handlerReadiness(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", " text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(http.StatusText(http.StatusOK)))
}

func (a *apiConfig) handlerValidateChirp(w http.ResponseWriter, r *http.Request) {
	header, textBody := decoder(w, r)
	if (header == 500) {
		return
	}
	header = encoder(w, r, header, textBody)
	if (header == 500) {
		log.Printf("Something went wrong.")
		return
	}
	return
}

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
	if (a.platform != "dev") {
		w.WriteHeader(403)
		forbiddenString := fmt.Sprintf("403 Access forbidden. For admin use only.\n")
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

func decoder(w http.ResponseWriter, r *http.Request) (int, string) {
    type parameters struct {
        Body string `json:"body"`
		Email string `json:"email"`
    }
	returnString := ""
    decoder := json.NewDecoder(r.Body)
    params := parameters{}
    err := decoder.Decode(&params)
	fmt.Printf("Message body: %s\n", params.Body)
	fmt.Printf("Email body: %s\n", params.Email)

	if (params.Body == "") {
		returnString = params.Email
	} else {
		returnString = params.Body
	}

	if ( len(params.Body) > 140 ) {
		log.Printf("Body too large.")
		return 400, returnString
	}
    if err != nil {
		log.Printf("Error decoding parameters: %s", err)
		return 500, returnString
    }
	return 200, returnString
}

func searchProfanity(text string) (string){
	badWords := []string{"kerfuffle", "sharbert", "fornax"}
	newText := strings.Split(text, " ")

	for i, originalWord := range(newText) {
		for _, badWord := range(badWords) {
			if (strings.ToLower(originalWord) == badWord) {
				newText[i] = "****"
			}
		}
	}
	
	words := strings.Join(newText, " ")

	return words
}

func encoder(w http.ResponseWriter, r *http.Request, header int, response string) (int){
    type returnValid struct {
        Valid bool `json:"valid"`
    }
	type returnError struct {
        Error string `json:"error"`
    }
	type returnCleanText struct {
		CleanedBody string `json:"cleaned_body"`
	}


	if (header == 400) {
		respBody := returnError{Error:"Chirp is too long"}
		dat, err := json.Marshal(respBody)
		if err != nil {
				log.Printf("Error marshalling JSON: %s", err)
				w.WriteHeader(500)
				return 500
		}
		w.Header().Set("Content-Type", "application/json")
    	w.WriteHeader(400)
    	w.Write(dat)
		return 400
	}
	if (header == 200) {
		cleanedResponse := searchProfanity(response)
		if header == (500) {
			log.Printf("Error decoding JSON: %s")
			return 500
		}
		respBody := returnCleanText{CleanedBody:cleanedResponse}
		dat, err := json.Marshal(respBody)
		if err != nil {
				log.Printf("Error marshalling JSON: %s", err)
				w.WriteHeader(500)
				return 500
		}
		w.Header().Set("Content-Type", "application/json")
    	w.WriteHeader(200)
    	w.Write(dat)
		return 200
	}
    
	return 500
}