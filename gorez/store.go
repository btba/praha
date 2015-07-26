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

type Store interface {
	ListOpenToursByCode() (map[string][]*Tour, error)
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
		var id int64
		var code sql.NullString
		var time time.Time
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
