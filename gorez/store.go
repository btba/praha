package main

import (
	"database/sql"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type Tour struct {
	ID   int64
	Code string
	Time time.Time
}

type CartItem struct {
	ID       int
	TourID   int
	Quantity int
}

type CartItemDetail struct {
	CartItem
	TourCode  string
	TourTime  time.Time
	TourPrice int
}

type Store interface {
	ListOpenToursByCode() (map[string][]*Tour, error)
	ListCartItems(cartID int) ([]*CartItem, error)
	AddCartItem(cartID, tourID, quantity int) error
	UpdateCartItem(cartID, itemID, quantity int) error
	DeleteCartItem(cartID, itemID int) error
	ListCartItemDetails(cartID int) ([]*CartItemDetail, error)
}

type RemoteStore struct {
	db *sql.DB
}

func (s *RemoteStore) ListOpenToursByCode() (map[string][]*Tour, error) {
	rows, err := s.db.Query("SELECT TourID, TourCode, TourDateTime "+
		"FROM Master "+
		"WHERE TourDateTime >= ? AND Public <> 0 AND Cancelled = 0 AND Deleted = 0 "+
		"ORDER BY TourDateTime ASC",
		time.Now().Format("2006-01-02 15:04:05"))
	if err != nil {
		return nil, err
	}
	toursByCode := make(map[string][]*Tour)
	for rows.Next() {
		var (
			id   int64
			code sql.NullString // TODO: Make this non-nullable?
			time time.Time
		)
		if err := rows.Scan(&id, &code, &time); err != nil {
			return nil, err
		}
		toursByCode[code.String] = append(toursByCode[code.String], &Tour{
			ID:   id,
			Code: code.String,
			Time: time,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return toursByCode, nil
}

func (s *RemoteStore) ListCartItems(cartID int) ([]*CartItem, error) {
	rows, err := s.db.Query("SELECT ItemPos, TourID, RiderCount "+
		"FROM CartItems "+
		"WHERE CartItems.CartID = ?",
		cartID)
	if err != nil {
		return nil, err
	}
	var items []*CartItem
	for rows.Next() {
		var itemPos, tourID, riderCount int
		if err := rows.Scan(&itemPos, &tourID, &riderCount); err != nil {
			return nil, err
		}
		items = append(items, &CartItem{
			ID:       itemPos,
			TourID:   tourID,
			Quantity: riderCount,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *RemoteStore) AddCartItem(cartID, tourID, quantity int) error {
	_, err := s.db.Exec(
		"INSERT INTO CartItems (CartID, TourID, RiderCount) VALUES (?, ?, ?)",
		cartID, tourID, quantity)
	return err
}

func (s *RemoteStore) UpdateCartItem(cartID, itemID, quantity int) error {
	_, err := s.db.Exec(
		"UPDATE CartItems SET RiderCount = ? WHERE CartID = ? AND ItemPos = ?",
		quantity, cartID, itemID)
	return err
}

func (s *RemoteStore) DeleteCartItem(cartID, itemID int) error {
	_, err := s.db.Exec(
		"DELETE FROM CartItems WHERE CartID = ? AND ItemPos = ?",
		cartID, itemID)
	return err
}

func (s *RemoteStore) ListCartItemDetails(cartID int) ([]*CartItemDetail, error) {
	rows, err := s.db.Query("SELECT CartItems.ItemPos, CartItems.TourID, CartItems.RiderCount, Master.TourCode, Master.TourDateTime, MasterTourInfo.Price "+
		"FROM CartItems, Master, MasterTourInfo "+
		"WHERE CartItems.CartID = ? AND CartItems.TourID = Master.TourID AND Master.TourCode = MasterTourInfo.ShortCode",
		cartID)
	if err != nil {
		return nil, err
	}
	var items []*CartItemDetail
	for rows.Next() {
		var (
			itemPos    int
			tourID     int
			riderCount int
			tourCode   string
			tourTime   time.Time
			tourPrice  float64
		)
		if err := rows.Scan(&itemPos, &tourID, &riderCount, &tourCode, &tourTime, &tourPrice); err != nil {
			return nil, err
		}
		items = append(items, &CartItemDetail{
			CartItem: CartItem{
				ID:       itemPos,
				TourID:   tourID,
				Quantity: riderCount,
			},
			TourCode:  tourCode,
			TourTime:  tourTime,
			TourPrice: int(100 * tourPrice),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
