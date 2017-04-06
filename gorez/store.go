package main

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

type Tour struct {
	ID            int32
	Code          string
	Time          time.Time
	AutoConfirm   bool
	HeightsNeeded bool
}

type TourDetail struct {
	Tour
	LongName          string
	Price             float64
	NumSpotsRemaining int
}

type Store interface {
	GetTourDetailByID(tourID int32, maxRiders int) (*TourDetail, bool, error)
	CreateOrder(tourID int32, genders []string, heights []int, total uint64, name, email, mobile, hotel, misc string) (int32, error)
	UpdateOrderPaymentRecorded(orderID int32) error
	UpdateOrderConfirmationSent(orderID int32) error
}

type RemoteStore struct {
	db *sql.DB
}

func (s *RemoteStore) GetTourDetailByID(tourID int32, maxRiders int) (*TourDetail, bool, error) {
	var (
		id            int32
		code          sql.NullString
		time          mysql.NullTime
		autoConfirm   sql.NullBool
		riderLimit    sql.NullInt64
		heightsNeeded sql.NullBool
		longName      sql.NullString
		price         sql.NullFloat64
		numRiders     sql.NullInt64 // SUM() can return NULL
	)
	row := s.db.QueryRow(""+
		"SELECT Master.TourID, "+
		"    Master.TourCode, "+
		"    Master.TourDateTime, "+
		"    Master.AutoConfirm <> 0, "+
		"    Master.RiderLimit, "+
		"    Master.HeightsNeeded IS NOT NULL, "+
		"    MasterTourInfo.LongName, "+
		"    MasterTourInfo.Price, "+
		"    Riders.Count "+
		"FROM Master "+
		"LEFT JOIN MasterTourInfo ON Master.TourCode = MasterTourInfo.ShortCode "+
		"LEFT JOIN ("+
		"    SELECT TourID, SUM(Riders) AS Count "+
		"    FROM OrderItems "+
		"    GROUP BY TourID"+
		") AS Riders ON Master.TourID = Riders.TourID "+
		"WHERE Master.TourID = ?",
		tourID)
	err := row.Scan(&id, &code, &time, &autoConfirm, &riderLimit, &heightsNeeded, &longName, &price, &numRiders)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, err
	}
	tourDetail := &TourDetail{
		Tour: Tour{
			ID:            id,
			Code:          code.String,
			Time:          time.Time,
			AutoConfirm:   autoConfirm.Bool,
			HeightsNeeded: heightsNeeded.Bool,
		},
		LongName: longName.String,
		Price:    price.Float64,
	}
	if !riderLimit.Valid || riderLimit.Int64 == 0 {
		tourDetail.NumSpotsRemaining = maxRiders
	} else {
		tourDetail.NumSpotsRemaining = int(riderLimit.Int64) - int(numRiders.Int64)
	}
	return tourDetail, true, nil
}

func priceString(total uint64) string {
	return fmt.Sprintf("%d", total/100)
}

func heightsString(genders []string, heights []int) string {
	var s []string
	for i, g := range genders {
		var h string
		if i < len(heights) {
			switch heights[i] {
			case 0, -1:
				h = "??"
			case 1:
				h = "<4'8"
			case 100:
				h = ">6'6"
			default:
				h = fmt.Sprintf("%d'%d", heights[i]/12, heights[i]%12)
			}
		}
		s = append(s, g+h)
	}
	return strings.Join(s, " ")
}

func (s *RemoteStore) prepareCreateOrder(tx *sql.Tx, tourID int32, genders []string, heights []int, total uint64, name, email, mobile, hotel, misc string) (int32, error) {
	result, err := tx.Exec(
		"INSERT INTO OrderMain (CustName, CustEmail, Hotel, Mobile, DatePlaced, Message, Heights) VALUES (?, ?, ?, ?, ?, ?, ?)",
		name, email, hotel, mobile, time.Now(), misc, heightsString(genders, heights))
	if err != nil {
		return 0, err
	}
	orderID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	_, err = tx.Exec(
		"INSERT INTO OrderItems (OrderNum, TourID, Riders, Price, Method) VALUES (?, ?, ?, ?, ?)",
		orderID, tourID, len(heights), priceString(total), "STw")
	if err != nil {
		return 0, err
	}
	return int32(orderID), nil
}

func (s *RemoteStore) CreateOrder(tourID int32, genders []string, heights []int, total uint64, name, email, mobile, hotel, misc string) (int32, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	orderID, err := s.prepareCreateOrder(tx, tourID, genders, heights, total, name, email, mobile, hotel, misc)
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

func (s *RemoteStore) UpdateOrderConfirmationSent(orderID int32) error {
	_, err := s.db.Exec(
		"UPDATE OrderItems SET ConfirmationSent = 1 WHERE OrderNum = ?", orderID)
	return err
}
