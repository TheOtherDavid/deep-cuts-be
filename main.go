package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	spot "github.com/TheOtherDavid/deep-cuts/internal/spotify"
	spotify "github.com/zmb3/spotify/v2"
)

type Code struct {
	Code string `json:"code"`
}

type TokenResponse struct {
	Token string `json:"token"`
}

func respondWithError(w http.ResponseWriter, code int, message string) {
	respondWithJSON(w, code, map[string]string{"error": message})
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}

func generateDeepCutPlaylist() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")

		tokenHeader := r.Header.Get("Authorization")
		if tokenHeader == "" {
			http.Error(w, "Couldn't get playlist ID from request.", http.StatusForbidden)
			return
		}

		log.Println("Token header is: " + string(tokenHeader))
		headerParts := strings.Split(tokenHeader, " ")
		token := headerParts[1]

		client, user, err := spot.GetAuthWithToken(token)
		if err != nil {
			http.Error(w, "Error getting auth.", http.StatusForbidden)
			log.Println(err)
			return
		}

		fmt.Println("Begin generate Deep Cut playlist.")

		playlistId := mux.Vars(r)["playlistId"]
		if playlistId == "" || playlistId == "undefined" {
			http.Error(w, "Couldn't get playlist ID from request.", http.StatusBadRequest)
			log.Println(err)
			return
		}

		ctx := context.Background()

		//Get input playlist
		playlist, err := getSpotifyPlaylist(ctx, client, playlistId)
		if err != nil {
			http.Error(w, "Error retrieving playlist.", http.StatusInternalServerError)
			log.Println(err)
			return
		}

		fmt.Println("Playlist retrieved.")
		//Get each track
		originalTracks := getFullTracksFromPlaylist(ctx, client, playlist)
		if len(originalTracks) == 0 {
			http.Error(w, "No tracks in playlist", http.StatusBadRequest)
			log.Println(err)
			return
		}
		fmt.Println("Tracks retrieved.")

		playlistName := playlist.Name
		newPlaylistName := playlistName + "-deep-cut"

		programMode := os.Getenv("PROGRAM_MODE")

		finalTracks, err := getFinalPlaylistTracks(ctx, client, originalTracks, programMode)
		if err != nil {
			http.Error(w, "Error determining playlist tracks.", http.StatusInternalServerError)
			log.Println(err)
			return
		}
		//Create playlist
		generatedPlaylistId, err := createPlaylist(ctx, client, user, finalTracks, newPlaylistName)
		if err != nil {
			http.Error(w, "Error creating playlist.", http.StatusInternalServerError)
			log.Println(err)
			return
		}

		//Get playlist from ID
		playlist, err = getSpotifyPlaylist(ctx, client, generatedPlaylistId)
		if err != nil {
			http.Error(w, "Error retrieving playlist.", http.StatusInternalServerError)
			log.Println(err)
			return
		}

		fmt.Println("Playlist retrieved.")

		//Get easier-to-process tracks from playlist
		playlistTracks := getFullTracksFromPlaylist(ctx, client, playlist)

		defer r.Body.Close()
		fmt.Println("Returning response to GeneratePlaylist.")

		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusCreated)

		json.NewEncoder(w).Encode(playlistTracks)
	}
}

func getPlaylist() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		log.Println("Assigned Access Control Header.")

		tokenHeader := r.Header.Get("Authorization")
		if tokenHeader == "" {
			http.Error(w, "Couldn't get playlist ID from request.", http.StatusForbidden)
		}

		log.Println("Token header is: " + string(tokenHeader))
		headerParts := strings.Split(tokenHeader, " ")
		token := headerParts[1]

		//Now we somehow get the client with the token instead of the code.
		client, _, err := spot.GetAuthWithToken(token)
		if err != nil {
			http.Error(w, "Error getting client.", http.StatusForbidden)
			log.Println(err)
			return
		}

		playlistId := mux.Vars(r)["playlistId"]
		if playlistId == "" || playlistId == "undefined" {
			http.Error(w, "Couldn't get playlist ID from request.", http.StatusBadRequest)
		}

		ctx := context.Background()

		//Get playlist from ID
		playlist, err := getSpotifyPlaylist(ctx, client, playlistId)
		if err != nil {
			http.Error(w, "Error retrieving playlist.", http.StatusInternalServerError)
			log.Println(err)
			return
		}

		fmt.Println("Playlist retrieved.")

		//Get easier-to-process tracks from playlist
		playlistTracks := getFullTracksFromPlaylist(ctx, client, playlist)

		defer r.Body.Close()

		fmt.Println("Returning response to GetPlaylist.")

		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)

		json.NewEncoder(w).Encode(playlistTracks)
	}
}

func getSpotifyPlaylist(ctx context.Context, client *spotify.Client, playlistId string) (*spotify.FullPlaylist, error) {
	fmt.Println("Beginning getSpotifyPlaylist")
	finalPlaylist := spotify.FullPlaylist{}

	spotifyPlaylistId := spotify.ID(playlistId)

	offset := 0
	limit := 100

	for {
		spotifyOffset := spotify.Offset(offset)
		playlist, err := client.GetPlaylist(ctx, spotifyPlaylistId, spotifyOffset, spotify.Limit(limit))

		if err != nil {
			return nil, err
		}
		fmt.Println(playlist.ID)

		if finalPlaylist.Name == "" {
			finalPlaylist.Name = playlist.Name
		}
		finalPlaylist.Tracks.Tracks = append(finalPlaylist.Tracks.Tracks, playlist.Tracks.Tracks...)

		if playlist.Tracks.Total <= offset+limit {
			break
		}
		offset = offset + limit
	}

	return &finalPlaylist, nil
}

func getFullTracksFromPlaylist(ctx context.Context, client *spotify.Client, playlist *spotify.FullPlaylist) []spotify.FullTrack {
	fmt.Println("Beginning getFullTracksFromPlaylist")
	tracks := []spotify.FullTrack{}
	playlistTracks := playlist.Tracks.Tracks
	for _, playlistTrack := range playlistTracks {
		tracks = append(tracks, playlistTrack.Track)
	}
	return tracks
}

func getAlbum(ctx context.Context, client *spotify.Client, albumId spotify.ID) (*spotify.FullAlbum, error) {
	fmt.Println("Beginning getAlbum")
	album, err := client.GetAlbum(ctx, albumId)

	if err != nil {
		return nil, err
	}
	fmt.Println(album.ID)
	return album, nil
}

func createPlaylist(ctx context.Context, client *spotify.Client, user *spotify.PrivateUser, tracks []spotify.SimpleTrack, playlistName string) (string, error) {
	fmt.Println("Beginning createPlaylist")

	playlistDescription := "Created automatically"
	createPublicPlaylist := false
	collaborativePlaylist := false
	userId := user.ID

	createdPlaylist, err := client.CreatePlaylistForUser(ctx, userId, playlistName, playlistDescription, createPublicPlaylist, collaborativePlaylist)
	if err != nil {
		return "", err
	}
	playlistId := createdPlaylist.ID
	//Get track IDs from list of tracks
	var trackIds []spotify.ID
	for _, track := range tracks {
		trackIds = append(trackIds, track.ID)
	}
	size := 100
	var j int

	//Batch trackIds in groups of 100 for being added to the playlist
	for i := 0; i < len(trackIds); i += size {
		j += size
		if j > len(trackIds) {
			j = len(trackIds)
		}
		fmt.Println(trackIds[i:j])
		_, err = client.AddTracksToPlaylist(ctx, playlistId, trackIds[i:j]...)
		if err != nil {
			fmt.Println(err.Error)
			return "", err
		}
	}

	fmt.Println("Playlist created")
	return playlistId.String(), nil
}

func getFinalPlaylistTracks(ctx context.Context, client *spotify.Client, originalTracks []spotify.FullTrack, programMode string) ([]spotify.SimpleTrack, error) {
	finalTracks := []spotify.SimpleTrack{}
	forbiddenSongs := []spotify.SimpleTrack{}
	rand.Seed(time.Now().UnixNano())

	//First we add every item on the list to the forbiddenSongs list
	for _, originalTrack := range originalTracks {
		forbiddenSongs = append(forbiddenSongs, originalTrack.SimpleTrack)
	}

	for i, originalTrack := range originalTracks {
		//Get album for each track, and the tracks from those albums
		albumId := originalTrack.Album.ID
		album, err := getAlbum(ctx, client, albumId)
		if err != nil {
			return nil, err
		}
		fmt.Println("Album retrieved: " + strconv.Itoa(i))
		albumTracklist := album.Tracks.Tracks
		switch programMode {
		case "ALL_BUT_ORIGINAL":
			//For each track on the album, add it to the final list if it isn't the original track
			for _, albumTrack := range albumTracklist {
				if !isSongIDForbidden(forbiddenSongs, albumTrack) {
					finalTracks = append(finalTracks, albumTrack)
				}
			}
		case "ONE_TRACK_PER_TRACK":
			//Go through the rest of the album's tracks and test for acceptable songs (which order?)
			//Figure out acceptable songs
			acceptableTracks := findAcceptableTracks(albumTracklist, forbiddenSongs)
			if len(acceptableTracks) > 0 {
				trackIndex := rand.Int() % len(acceptableTracks)
				trackToAdd := acceptableTracks[trackIndex]
				finalTracks = append(finalTracks, trackToAdd)
				forbiddenSongs = append(forbiddenSongs, trackToAdd)
			}
		}
	}
	return finalTracks, nil
}

func isSongIDForbidden(forbiddenSongs []spotify.SimpleTrack, song spotify.SimpleTrack) bool {
	return contains(forbiddenSongs, song)
}

func contains(s []spotify.SimpleTrack, e spotify.SimpleTrack) bool {
	for _, a := range s {
		if a.ID == e.ID {
			return true
		}
	}
	return false
}

func findAcceptableTracks(potentialTracks []spotify.SimpleTrack, forbiddenTracks []spotify.SimpleTrack) []spotify.SimpleTrack {
	acceptableTracks := []spotify.SimpleTrack{}
	//Loop through list One
	for _, potentialTrack := range potentialTracks {
		addTrack := true
		//Loop through list Two
		for _, forbiddenTrack := range forbiddenTracks {
			if potentialTrack.ID == forbiddenTrack.ID {
				//If an item in the potentialTrack exists in the forbiddenTrack list, do not add it to the acceptableTrack list.
				addTrack = false
				break
			}
		}
		if addTrack == true {
			acceptableTracks = append(acceptableTracks, potentialTrack)
		}
	}
	return acceptableTracks
}

func getSpotifyToken() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("Beginning Get Token function.")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		codes, ok := r.URL.Query()["code"]

		if !ok || len(codes[0]) < 1 {
			http.Error(w, "Couldn't get code from request.", http.StatusForbidden)
			log.Println("Couldn't get code from request.")
			return
		}

		code := codes[0]

		log.Println("Url Param 'code' is: " + string(code))

		token, err := spot.GetTokenWithCode(code)
		if err != nil {
			http.Error(w, "Error getting token.", http.StatusForbidden)
			log.Println(err)
			return
		}
		tokenResponse := TokenResponse{
			Token: token,
		}

		w.WriteHeader(http.StatusOK)
		log.Println("Successfully returning token.")

		json.NewEncoder(w).Encode(tokenResponse)
	}
}

type healthCheckResponse struct {
	Status string `json:"status"`
}

func health() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")

		response := healthCheckResponse{
			Status: "Ok",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}

func cors() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept, Authorization")
		w.WriteHeader(http.StatusOK)
	}
}

func handleRequests() {
	log.Println("Creating Router")
	myRouter := mux.NewRouter().StrictSlash(true)
	myRouter.HandleFunc("/callback", spot.CompleteAuth)
	myRouter.HandleFunc("/health", health()).Methods("GET")
	myRouter.HandleFunc("/token", getSpotifyToken()).Methods("GET")
	myRouter.HandleFunc("/{playlistId}", getPlaylist()).Methods("GET")
	myRouter.HandleFunc("/{playlistId}", generateDeepCutPlaylist()).Methods("POST")
	myRouter.HandleFunc("/{playlistId}", cors()).Methods("OPTIONS")
	myRouter.HandleFunc("/", cors()).Methods("OPTIONS")

	log.Fatal(http.ListenAndServe(":8080", myRouter))
}

func main() {
	fmt.Println("Listening on port 8080")
	handleRequests()
}
