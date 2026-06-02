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

	"github.com/finfreezer/Chirpy/internal/auth"
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
	secret         string
}

type User struct {
	ID           uuid.UUID `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Email        string    `json:"email"`
	IsRed        bool      `json:"is_chirpy_red"`
	Token        string    `json:"token,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Password     string    `json:"password,omitempty"`
}

type ChirpBody struct {
	Body string `json:"body"`
}

type UserParameters struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type returnError struct {
	Error string `json:"error"`
}

type upgradeEvent struct {
	Event string `json:"event"`
	Data  struct {
		UserID string `json:"user_id"`
	} `json:"data"`
}

// curl -X POST -H "Content-Type: application/json" -d '{"email":"protector@g.com","password":"securePassword"}' http://localhost:8080/api/users
// curl -X POST -H "Content-Type: application/json" -d '{"email":"protector@g.com","password":"securePassword"}' http://localhost:8080/api/login
// curl -X POST -H "Content-Type: application/json" -d '{"email":"protector@g.com","password":"securePassword","expires_in_seconds":60}' http://localhost:8080/api/login
func main() {
	auth.CreatePasswordHash("123")
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	platform := os.Getenv("PLATFORM")
	secret := os.Getenv("SECRET")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	dbQueries := database.New(db)
	a := &apiConfig{atomic.Int32{}, dbQueries, platform, secret}
	newMux := http.NewServeMux()
	newMux.Handle("/app/", a.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))
	newMux.HandleFunc("GET /api/healthz", handlerReadiness)
	newMux.HandleFunc("GET /admin/metrics", a.handlerWriteMetrics)
	newMux.HandleFunc("POST /admin/reset", a.handlerResetMetrics)
	//newMux.HandleFunc("POST /api/validate_chirp", a.handlerValidateChirp)
	newMux.HandleFunc("POST /api/users", a.handlerCreateUser)
	newMux.HandleFunc("POST /api/chirps", a.handlerChirp)
	newMux.HandleFunc("GET /api/chirps", a.handlerGetChirps)
	newMux.HandleFunc("GET /api/chirps/{id}", a.handlerGetChirpByID)
	newMux.HandleFunc("POST /api/login", a.handlerLogin)
	newMux.HandleFunc("POST /api/refresh", a.handlerRefresh)
	newMux.HandleFunc("POST /api/revoke", a.handlerRevoke)
	newMux.HandleFunc("PUT /api/users", a.handlerUpdateUser)
	newMux.HandleFunc("DELETE /api/chirps/{chirpID}", a.handlerDeleteChirp)
	newMux.HandleFunc("POST /api/polka/webhooks", a.handlerUpgradeEvent)
	newServer := http.Server{Addr: ":8080", Handler: newMux}
	err = newServer.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}

func (a *apiConfig) handlerUpgradeEvent(w http.ResponseWriter, r *http.Request) {
	resp := decoder(r, "UpgradeEvent")
	if resp.Status == 204 || resp.Status == 500 {
		log.Println("Error: invalid event.")
		w.WriteHeader(resp.Status)
		return
	}

	event := upgradeEvent{}
	err := json.Unmarshal(*resp.Data, &event)
	if err != nil {
		log.Printf("Error unmarshaling event data: %s\n", err)
		w.WriteHeader(500)
		return
	}
	user, err := a.database.GetUserByRFToken(context.Background(),
		uuid.MustParse(event.Data.UserID))
	if err != nil {
		log.Printf("Error finding user from database: %s\n", err)
		w.WriteHeader(404)
		return
	}

	_, err = a.database.UpgradeUserToRed(context.Background(), user.ID)

	if err != nil {
		log.Printf("Error upgrading user to Red: %s\n", err)
		w.WriteHeader(500)
		return
	}

	w.WriteHeader(204)
}

func (a *apiConfig) handlerDeleteChirp(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("chirpID")
	chirp, err := a.database.GetChirpByID(context.Background(), uuid.MustParse(id))
	if err != nil {
		log.Printf("Error finding Chirp based on ID: %s\n", err)
		w.WriteHeader(404)
		return
	}

	accessToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("Error getting access token: %s\n", err)
		w.WriteHeader(403)
		return
	}

	userID, err := auth.ValidateJWT(accessToken, a.secret)
	if err != nil {
		log.Printf("Error validating token: %s\n", err)
		w.WriteHeader(401)
		return
	}

	if chirp.UserID != userID {
		log.Printf("IDs don't match.\n")
		w.WriteHeader(403)
		return
	}
	params := database.DeleteChirpParams{ID: chirp.ID, UserID: userID}
	err = a.database.DeleteChirp(r.Context(), params)
	if err != nil {
		log.Printf("Error deleting Chirp from database: %s\n", err)
		w.WriteHeader(500)
		return
	}
	w.WriteHeader(204)
}

func (a *apiConfig) handlerUpdateUser(w http.ResponseWriter, r *http.Request) {
	accessToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("Error getting access token: %s\n", err)
		w.WriteHeader(401)
		return
	}
	resp := decoder(r, "CreateUser")
	if resp.Status != 200 {
		log.Printf("Error decoding request body.\n")
		w.WriteHeader(resp.Status)
	}
	userInfo := UserParameters{}
	err = json.Unmarshal(*resp.Data, &userInfo)
	if err != nil {
		log.Printf("Error unmarshaling response in UpdateUser: %s\n", err)
		w.WriteHeader(401)
		return
	}
	userID, err := auth.ValidateJWT(accessToken, a.secret)
	if err != nil {
		log.Printf("Error validating JWT in UpdateUser: %s\n", err)
		w.WriteHeader(401)
		return
	}
	hashedPW, err := auth.CreatePasswordHash(userInfo.Password)
	if err != nil {
		log.Printf("Error hashing password in UpdateUser: %s\n", err)
		w.WriteHeader(401)
		return
	}
	params := database.UpdateUserParams{
		HashedPassword: hashedPW,
		Email:          userInfo.Email,
		ID:             userID,
	}
	user, err := a.database.UpdateUser(context.Background(), params)
	if err != nil {
		log.Printf("Error updating user in database: %s\n", err)
		w.WriteHeader(401)
		return
	}

	responseUser := User{
		ID:        user.ID,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		Email:     user.Email,
		IsRed:     user.IsChirpyRed,
		Token:     accessToken}

	dat, err := json.Marshal(&responseUser)
	if err != nil {
		log.Printf("Error marshaling user in UpdateUser: %s\n", err)
		w.WriteHeader(401)
		return
	}
	w.WriteHeader(200)
	w.Write(dat)

}

func (a *apiConfig) handlerRevoke(w http.ResponseWriter, r *http.Request) {
	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("Error finding refresh token: %s\n", err)
		w.WriteHeader(401)
		return
	}
	DbToken, err := a.database.GetRefreshToken(context.Background(), refreshToken)
	if err != nil {
		log.Printf("No token in database: %s\n", err)
		w.WriteHeader(401)
		return
	}
	newNulltime := sql.NullTime{Time: time.Now(), Valid: true}
	params := database.UpdateRFTokenParams{RevokedAt: newNulltime, UpdatedAt: time.Now(), Token: DbToken.Token}
	err = a.database.UpdateRFToken(context.Background(), params)
	if err != nil {
		log.Printf("Error updating refresh token database entry.\n")
		w.WriteHeader(401)
		return
	}
	w.WriteHeader(204)

}

func (a *apiConfig) handlerRefresh(w http.ResponseWriter, r *http.Request) {
	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("Error finding refresh token: %s\n", err)
		w.WriteHeader(401)
		return
	}
	dbToken, err := a.database.GetRefreshToken(context.Background(), refreshToken)
	if err != nil {
		log.Printf("No token in database: %s\n", err)
		w.WriteHeader(401)
		return
	}
	user, err := a.database.GetUserByRFToken(context.Background(), dbToken.UserID)
	if err != nil {
		log.Printf("No user found for token ID: %s\n", err)
		w.WriteHeader(401)
		return
	}
	jwtToken, err := auth.MakeJWT(user.ID, a.secret, 60*time.Minute)
	if err != nil {
		log.Printf("Error creating JWT for current user: %s\n", err)
		w.WriteHeader(401)
		return
	}

	if time.Now().After(dbToken.ExpiresAt) {
		log.Printf("Token expired\n")
		w.WriteHeader(401)
		return
	}
	if dbToken.RevokedAt.Valid {
		log.Printf("Token revoked.\n")
		w.WriteHeader(401)
		return
	}
	type tokenResp struct {
		Token string `json:"token"`
	}
	resp := tokenResp{Token: jwtToken}
	response, err := json.Marshal(resp)
	if err != nil {
		log.Printf("Error marshaling refresh token: %s\n", err)
		w.WriteHeader(401)
		return
	}
	w.WriteHeader(200)
	w.Write(response)
}

func (a *apiConfig) handlerLogin(w http.ResponseWriter, r *http.Request) {
	resp := decoder(r, "CreateUser")
	userInfo := UserParameters{}
	err := json.Unmarshal(*resp.Data, &userInfo)
	user, err := a.database.GetUserByEmail(context.Background(), userInfo.Email)

	if err != nil {
		log.Printf("Error fetching user: %s\n", err)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Incorrect email or password"))
		return
	}

	if ok, _ := auth.CheckPasswordHash(userInfo.Password, user.HashedPassword); ok {
		authToken, err := auth.MakeJWT(user.ID, a.secret, time.Duration(3600*time.Second))

		if err != nil {
			log.Printf("Problem forming JWT: %s\n", err)
			w.WriteHeader(500)
			return
		}
		refreshToken := auth.MakeRefreshToken()
		err = a.helperAddRefreshToken(refreshToken, user.ID)
		if err != nil {
			log.Printf("Error adding refresh token to database: %s\n", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		responseUser := User{
			ID:           user.ID,
			CreatedAt:    user.CreatedAt,
			UpdatedAt:    user.UpdatedAt,
			Email:        user.Email,
			IsRed:        user.IsChirpyRed,
			Token:        authToken,
			RefreshToken: refreshToken}
		dat, err := json.Marshal(&responseUser)
		if err != nil {
			log.Println("Problem marshaling user.")
			w.WriteHeader(500)
			return
		}
		w.Write(dat)
		return
	}

	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte("Incorrect email or password"))
}

func (a *apiConfig) helperAddRefreshToken(token string, userID uuid.UUID) error {
	revokeTime := sql.NullTime{Time: time.Time{}, Valid: false}
	expiry, err := time.ParseDuration("1440h")
	expireTime := time.Now().Add(expiry)
	if err != nil {
		log.Printf("Error creating Refresh Token expiration duration: %s\n", err)
	}
	newRefreshToken := database.CreateRefreshTokenParams{
		Token:     token,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		UserID:    userID,
		ExpiresAt: expireTime,
		RevokedAt: revokeTime}
	_, err = a.database.CreateRefreshToken(context.Background(), newRefreshToken)
	if err != nil {
		log.Printf("Problem adding refresh token to Database: %s\n", err)
		return err
	}
	return nil
}

func (a *apiConfig) handlerGetChirpByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	chirp, err := a.database.GetChirpByID(context.Background(), uuid.MustParse(id))

	if err != nil {
		log.Printf("Error finding Chirp based on ID: %s\n", err)
		w.WriteHeader(404)
		return
	}
	chirpReg := buildChirpHelper(chirp)
	dat, err := json.Marshal(chirpReg)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(dat)
}

func (a *apiConfig) handlerGetChirps(w http.ResponseWriter, r *http.Request) {
	chirps, err := a.database.GetChirps(context.Background())
	chirArr := []Chirp{}
	if err != nil {
		log.Printf("Error getting chirps: %s\n", err)
	}
	for _, chirp := range chirps {
		chirArr = append(chirArr, buildChirpHelper(chirp))
	}
	dat, err := json.Marshal(chirArr)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(dat)
}

func buildChirpHelper(chirp database.Chirp) Chirp {
	regChirp := Chirp{ID: chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserID:    chirp.UserID,
	}
	return regChirp

}

func (a *apiConfig) handlerChirp(w http.ResponseWriter, r *http.Request) {
	resp := decoder(r, "PostChirp")
	if resp.Status == 500 || resp.Status == 400 {
		return
	}
	helperPostChirp(w, r, &resp, a)
	if resp.Status == 500 {
		log.Printf("Something went wrong.\n")
		return
	}
	/*chirp := Chirp{}
	json.Unmarshal(*resp.Data, &chirp)*/

}

func (a *apiConfig) handlerCreateUser(w http.ResponseWriter, r *http.Request) {

	resp := decoder(r, "CreateUser")
	userInfo := UserParameters{}
	err := json.Unmarshal(*resp.Data, &userInfo)
	if userInfo.Password == "" {
		w.WriteHeader(500)
		w.Write([]byte("Password required."))
		return
	}
	hashedPassword, err := auth.CreatePasswordHash(userInfo.Password)
	userParams := database.CreateUserParams{Email: userInfo.Email, HashedPassword: hashedPassword}
	newUser, err := a.database.CreateUser(context.Background(), userParams)
	if err != nil {
		log.Printf("Error creating user: %s\n", err)
	}
	responseUser := User{ID: newUser.ID,
		CreatedAt: newUser.CreatedAt,
		UpdatedAt: newUser.UpdatedAt,
		Email:     newUser.Email,
		IsRed:     newUser.IsChirpyRed}
	if resp.Status == 500 {
		log.Printf("Something went wrong decoding the email.\n")
		return
	}

	dat, err := json.Marshal(responseUser)
	if err != nil {
		log.Printf("Error marshalling JSON: %s\n", err)
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

// Possible requests: PostChirp, CreateUser, UpgradeEvent
func decoder(r *http.Request, Type string) RespJson {

	resp := RespJson{}
	var err error = nil
	decoder := json.NewDecoder(r.Body)
	err = decoder.Decode(&resp.Data)

	switch Type {

	case "CreateUser":
		if err != nil {
			log.Printf("Error decoding data for user: %s\n", err)
			resp.Status = 500
			return resp
		}
		resp.Status = 200
		return resp

	case "PostChirp":
		chirpText := ChirpBody{}
		json.Unmarshal(*resp.Data, &chirpText)
		if err != nil {
			log.Printf("Error decoding data for chirp: %s\n", err)
			resp.Status = 500
			return resp
		}

		if len(chirpText.Body) > 140 {
			log.Printf("Body too large.\n")
			resp.Status = 400
			return resp
		}
		resp.Status = 200
		return resp

	case "UpgradeEvent":
		event := upgradeEvent{}
		json.Unmarshal(*resp.Data, &event)
		if err != nil {
			log.Printf("Error decoding webhook data: %s\n", err)
			resp.Status = 500
			return resp
		}
		if event.Event != "user.upgraded" {
			resp.Status = 204
			return resp
		}
		resp.Status = 200
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

func helperPostChirp(w http.ResponseWriter, r *http.Request, data *RespJson, a *apiConfig) {

	authToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("Error receiving header data from request: %s\n", err)
		w.WriteHeader(401)
		return
	}
	userUUUID, err := auth.ValidateJWT(authToken, a.secret)
	if err != nil {
		log.Printf("Error validating JWT from request: %s\n", err)
		w.WriteHeader(401)
		return
	}

	if data.Status == 400 {
		respBody := returnError{Error: "Chirp is too long"}
		dat, err := json.Marshal(respBody)
		if err != nil {
			log.Printf("Error marshalling JSON: %s\n", err)
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
			log.Printf("Error unmarshalling JSON in helperPostChirp: %s\n", err)
		}
		cleanedResponse := searchProfanity(body.Body)
		if data.Status == 500 {
			log.Printf("Error decoding JSON\n")
			return
		}
		chirp := Chirp{}
		json.Unmarshal(*data.Data, &chirp)
		chirp.Body = cleanedResponse
		params := database.CreateChirpyParams{Body: chirp.Body, UserID: userUUUID}
		chirp2, err := a.database.CreateChirpy(context.Background(), params)
		combineChirps(&chirp, &chirp2)
		dat, err := json.Marshal(&chirp)
		if err != nil {
			log.Printf("Error marshalling JSON: %s\n", err)
			w.WriteHeader(500)
			data.Status = 500
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		w.Write(dat)
		data.Status = 201
		return
	}

	log.Printf("Warning: End of PostChirp reached.\n")
	data.Status = 500
}

func combineChirps(finalChirp *Chirp, dtbChirp *database.Chirp) {
	finalChirp.CreatedAt = dtbChirp.CreatedAt
	finalChirp.UpdatedAt = dtbChirp.UpdatedAt
	finalChirp.UserID = dtbChirp.UserID
	finalChirp.ID = dtbChirp.ID
}
