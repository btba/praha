package main

import (
	"math/rand"
	"net/http"
	"strconv"
)

const (
	cartIDCookieName = "BTBACartID"
)

func newCartID() int {
	// NB: Very small possibility of collision, so you would get an
	// existing cart.  It's ok.
	// TODO: Use an int64.
	return rand.Int() & 0xFFFFFFFF
}

func readCartID(r *http.Request) (int, bool) {
	cookie, err := r.Cookie(cartIDCookieName)
	if err != nil {
		return 0, false
	}
	cartID, err := strconv.Atoi(cookie.Value)
	if err != nil {
		return 0, false
	}
	return cartID, true
}

func writeCartID(w http.ResponseWriter, cartID int) {
	http.SetCookie(w, &http.Cookie{
		Name:  cartIDCookieName,
		Value: strconv.Itoa(cartID),
		Path:  "/",
	})
}
