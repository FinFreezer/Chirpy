# Chirpy
A programming exercise meant to practice databases, authentification, authorization and HTTP servers resembling Twitter.

## Requirements
This project requires

- [PostgreSQL](https://www.postgresql.org/)
- [Go](https://go.dev/doc/install)

On Linux: `sudo apt install postgresql postgresql-contrib`

For Go on Linux, you can also use [webinstall](https://webinstall.dev/golang/)

## Installation
```bash
go install github.com/finfreezer/Chirpy@latest
```

## Configuration
Set up a configuration file in your home directory called .env that includes the 

DB_URL 

> The connection URL to the database of your choice. e.g. postgres://user:password@localhost:5432/{database}?{sslmode=disable}

PLATFORM 

> "dev" by default for developed access.

SECRET 

> a random string used to verify the user, such as calling `openssl rand -base64 64` on a command line.

POLKA_KEY 

> A random string also used for authorization.

Example (With randomized values):
```
DB_URL="postgres://postgres:postgres@localhost:5432/chirpy?sslmode=disable"
PLATFORM="dev"
SECRET="I43OXKqj0Rlh7C16f8qr6EGYQcDk9Y+Tg/9f6PTn7cUx2a54pOKXf25usMf4sTHWsatrkJzN/qGa0f2aJhVlAA?!"
POLKA_KEY="mKjSfRg95ZcX7nQtbG+03gge"
```

## Usage

### The API

### Admin commands
If your .env file is configured as "dev", and includes the SECRET and POLKA_KEY, you have access to
the following api points:
- `GET /api/healthz` for checking if the server is running.
- `GET /admin/metrics` for checking amount of page views.
- `POST /admin/reset` to reset both page views, and current databases.
- `POST /api/polka/webhooks` to upgrade a user to "Chirpy Red" membership (currently no real function.)

### Global commands
You can start off by creating a **POST** request to /api/users with email and password e.g. using curl ` curl -X POST -H "Content-Type: application/json" -d '{"email":"protector@g.com","password":"securePassword"}' {DB_URL}/api/users `
This adds the given user to the Database. The password is encrypted and hashed using **argon2id** server-side.

To switch between users, you can use the **POST** request to the login API to log-in e.g. ` curl -X POST -H "Content-Type: application/json" -d '{"email":"protector@g.com","password":"securePassword"}' {DB_URL}/api/login `

Once you're logged-in, the server keeps track of the user through JWTs.
A user is logged-in for 1 hour at a time, but you can refresh the token by making a **POST** request
to `/api/refresh` while logged-in. A refresh token is 'alive' for 60 days.

You can revoke your own token by making a **POST** request to `/api/revoke`

To create a new Chirp, while logged-in, make a **POST** request to `/api/chirps` with a payload consisting of the Chirp's "body" e.g. ` curl -X POST -H "Content-Type: application/json" -d '{"body":"My first chirp!"}' {DB_URL}/api/chirps ` The Chirps have a character limit of 140.

To get a list of Chirps, create a **GET** request to `/api/chirps` without any special payload. You can request Chirps by a specific user by adding a user_id in the query e.g. `/api/chirps?author_id={id}` or sort them either in ascending or descending order based on date with `/api/chirps?sort={asc|desc}`

You can get a Chirp with a specific ID by simply adding it to the **GET** API request e.g. `/api/chirps/{id}`

To delete a Chirp, you must be logged in, and provide the ID of the Chirp that was authored by the currently logged-in user, and make a **DELETE** request to `/api/chirps/{chirpID}`
