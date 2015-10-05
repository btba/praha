package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
)

const (
	cartIDCookieName = "BTBACartID"
)

func newCartID() int64 {
	// NB: Very small possibility of collision, so you would get an
	// existing cart.  It's ok.
	return rand.Int63()
}

func readCartID(r *http.Request) (int64, bool) {
	cookie, err := r.Cookie(cartIDCookieName)
	if err != nil {
		return 0, false
	}
	cartID, err := strconv.ParseInt(cookie.Value, 16, 64)
	if err != nil {
		return 0, false
	}
	return cartID, true
}

func writeCartID(w http.ResponseWriter, cartID int64) {
	http.SetCookie(w, &http.Cookie{
		Name:  cartIDCookieName,
		Value: fmt.Sprintf("%016x", cartID),
		Path:  "/",
	})
}
