package client

import "github.com/google/uuid"

type Client interface {
	// TODO: add other options as necessary using functional options
	Connect(trtype string, subnqn string, traddr string, trsvcid string,
		hostnqn string) error
	GetDevPathByNGUID(nguid uuid.UUID) (string, error)
}
