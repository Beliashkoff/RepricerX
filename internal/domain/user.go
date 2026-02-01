package domain

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID          uuid.UUID `db:"id"`
	Fname       string    `db:"fname"`
	Sname       string    `db:"sname"`
	Tel         string    `db:"tel"`
	Email       string    `db:"email"`
	RoleType    string    `db:"role_type"`
	UtmSource   string    `db:"utm_source"`
	LastLoginAt time.Time `db:"last_login_at"`
	CreatedAt   time.Time `db:"created_at"`
}
