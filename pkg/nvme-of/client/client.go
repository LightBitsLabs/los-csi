// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package client

import "github.com/google/uuid"

type Client interface {
	// TODO: add other options as necessary using functional options
	Connect(trtype string, subnqn string, traddr string, trsvcid string,
		hostnqn string) error
	GetDevPathByNGUID(nguid uuid.UUID) (string, error)
}
