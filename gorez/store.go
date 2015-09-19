package main

import (
	"database/sql"
	"errors"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type Tour struct {
	ID   int32
	Code string
	Time time.Time
}

type TourDetail struct {
	Tour
	Price float64 // TODO: int32
}

type CartItem struct {
	ID       int32 // TODO: int64
	TourID   int32
	Quantity int32
}

type CartItemDetail struct {
	CartItem
	TourCode  string
	TourTime  time.Time
	TourPrice float64 // TODO: int32
}

type OrderItem struct {
	TourID   int32
	Quantity int32
}

type Store interface {
	ListOpenToursByCode() (map[string][]*Tour, error)
	GetTourDetailsByID(tourIDs []int32) (map[int32]*TourDetail, error)

	ListCartItems(cartID int32) ([]*CartItem, error)
	AddCartItem(cartID, tourID, quantity int32) error
	UpdateCartItem(cartID, itemID, quantity int32) error
	DeleteCartItem(cartID, itemID int32) error
	ListCartItemDetails(cartID int32) ([]*CartItemDetail, error)

	CreateOrder(name, email, mobile, hotel string, items []*OrderItem) (int32, error)
	UpdateOrderPaymentRecorded(orderID int32) error
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
			id   int32
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

func (s *RemoteStore) GetTourDetailsByID(tourIDs []int32) (map[int32]*TourDetail, error) {
	if len(tourIDs) == 0 {
		return nil, errors.New("Need at least one tour ID")
	}
	qmarks := "(?"
	for i := 1; i < len(tourIDs); i++ {
		qmarks += ", ?"
	}
	qmarks += ")"

	var tourIDargs []interface{}
	for _, x := range tourIDs {
		tourIDargs = append(tourIDargs, x)
	}
	rows, err := s.db.Query(
		"SELECT Master.TourID, Master.TourCode, Master.TourDateTime, MasterTourInfo.Price "+
			"FROM Master, MasterTourInfo "+
			"WHERE Master.TourID IN "+qmarks+" AND Master.TourCode = MasterTourInfo.ShortCode",
		tourIDargs...)
	if err != nil {
		return nil, err
	}
	tourDetails := make(map[int32]*TourDetail)
	for rows.Next() {
		var (
			id    int32
			code  sql.NullString // TODO: Make this non-nullable?
			time  time.Time
			price float64
		)
		if err := rows.Scan(&id, &code, &time, &price); err != nil {
			return nil, err
		}
		tourDetails[id] = &TourDetail{
			Tour: Tour{
				ID:   id,
				Code: code.String,
				Time: time,
			},
			Price: price,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tourDetails, nil
}

func (s *RemoteStore) ListCartItems(cartID int32) ([]*CartItem, error) {
	rows, err := s.db.Query("SELECT ItemPos, TourID, RiderCount "+
		"FROM CartItems "+
		"WHERE CartItems.CartID = ?",
		cartID)
	if err != nil {
		return nil, err
	}
	var items []*CartItem
	for rows.Next() {
		var (
			itemPos    int32
			tourID     int32
			riderCount int32
		)
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

func (s *RemoteStore) AddCartItem(cartID, tourID, quantity int32) error {
	_, err := s.db.Exec(
		"INSERT INTO CartItems (CartID, TourID, RiderCount) VALUES (?, ?, ?)",
		cartID, tourID, quantity)
	return err
}

func (s *RemoteStore) UpdateCartItem(cartID, itemID, quantity int32) error {
	_, err := s.db.Exec(
		"UPDATE CartItems SET RiderCount = ? WHERE CartID = ? AND ItemPos = ?",
		quantity, cartID, itemID)
	return err
}

func (s *RemoteStore) DeleteCartItem(cartID, itemID int32) error {
	_, err := s.db.Exec(
		"DELETE FROM CartItems WHERE CartID = ? AND ItemPos = ?",
		cartID, itemID)
	return err
}

func (s *RemoteStore) ListCartItemDetails(cartID int32) ([]*CartItemDetail, error) {
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
			itemPos    int32
			tourID     int32
			riderCount int32
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
			TourPrice: tourPrice,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *RemoteStore) prepareCreateOrder(tx *sql.Tx, name, email, hotel, mobile string, items []*OrderItem) (int32, error) {
	result, err := tx.Exec(
		"INSERT INTO OrderMain (CustName, CustEmail, Hotel, Mobile, DatePlaced) VALUES (?, ?, ?, ?, ?)",
		name, email, hotel, mobile, time.Now())
	if err != nil {
		return 0, err
	}
	orderID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	for i, item := range items {
		_, err := tx.Exec(
			"INSERT INTO OrderItems (OrderNum, ItemNum, TourID, Riders) VALUES (?, ?, ?, ?)",
			orderID, i, item.TourID, item.Quantity)
		if err != nil {
			return 0, err
		}
	}
	return int32(orderID), nil
}

func (s *RemoteStore) CreateOrder(name, email, hotel, mobile string, items []*OrderItem) (int32, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	orderID, err := s.prepareCreateOrder(tx, name, email, hotel, mobile, items)
	if err != nil {
		tx.Rollback()
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return orderID, nil
}

func (s *RemoteStore) UpdateOrderPaymentRecorded(orderID int32) error {
	_, err := s.db.Exec(
		"UPDATE OrderMain SET Completed = true WHERE OrderNum = ?", orderID)
	return err
}
