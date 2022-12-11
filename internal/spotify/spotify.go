package spotify

import (
	"context"
	"fmt"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"log"
	"net/http"

	"github.com/zmb3/spotify/v2"
	"golang.org/x/oauth2"
)

const redirectURI = "http://localhost:8080/callback"

//This is the URL of the UI
const redirectURIUI = "http://localhost:3000/"

type Code struct {
	Code string
}

var (
	auth = spotifyauth.New(
		spotifyauth.WithRedirectURL(redirectURI),
		spotifyauth.WithScopes(spotifyauth.ScopePlaylistModifyPrivate, spotifyauth.ScopePlaylistModifyPublic),
	)
	authUI = spotifyauth.New(
		spotifyauth.WithRedirectURL(redirectURIUI),
		spotifyauth.WithScopes(spotifyauth.ScopePlaylistModifyPrivate, spotifyauth.ScopePlaylistModifyPublic),
	)
	ch    = make(chan *spotify.Client)
	state = "abc123"
)

func GetAuth() (*spotify.Client, *spotify.PrivateUser) {
	var client *spotify.Client

	url := authUI.AuthURL(state)
	fmt.Println("Please log in to Spotify by visiting the following page in your browser:", url)

	// wait for auth to complete
	client = <-ch

	// use the client to make calls that require authorization
	user, err := client.CurrentUser(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("You are logged in as:", user.ID)

	return client, user
}

func CompleteAuth(w http.ResponseWriter, r *http.Request) {
	tok, err := auth.Token(r.Context(), state, r)
	if err != nil {
		http.Error(w, "Couldn't get token", http.StatusForbidden)
		log.Fatal(err)
	}
	if st := r.FormValue("state"); st != state {
		http.NotFound(w, r)
		log.Fatalf("State mismatch: %s != %s\n", st, state)
	}
	// use the token to get an authenticated client
	client := spotify.New(auth.Client(r.Context(), tok))
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, "Login Completed!")
	ch <- client
}

func GetAuthWithCode(code string) (*spotify.Client, *spotify.PrivateUser, error) {

	ctx := context.Background()

	token, err := authUI.Exchange(ctx, code)

	// wait for auth to complete
	//client = <-ch

	// use the token to get an authenticated client
	client := spotify.New(authUI.Client(ctx, token))

	// use the client to make calls that require authorization
	user, err := client.CurrentUser(context.Background())

	if err != nil {
		return nil, nil, err
	}
	fmt.Println("You are logged in as:", user.ID)

	return client, user, nil
}

func GetAuthWithToken(token string) (*spotify.Client, *spotify.PrivateUser, error) {
	ctx := context.Background()

	oauthToken := &oauth2.Token{
		AccessToken: token,
	}

	// use the token to get an authenticated client
	client := spotify.New(authUI.Client(ctx, oauthToken))

	// use the client to make calls that require authorization
	user, err := client.CurrentUser(ctx)

	if err != nil {
		return nil, nil, err
	}
	fmt.Println("You are logged in as:", user.ID)

	return client, user, nil
}

func GetTokenWithCode(code string) (string, error) {

	ctx := context.Background()

	token, err := authUI.Exchange(ctx, code)
	if err != nil {
		return "", err
	}

	tokenString := token.AccessToken

	return tokenString, err
}
