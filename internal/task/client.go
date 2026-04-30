package task

import (
	"encoding/json"

	"github.com/hibiken/asynq"
)

const (
	TypeCallbackA      = "callback:a"
	TypeCheckAbandoned = "check:abandoned"
)

type CallbackPayload struct {
	OrderID uint   `json:"order_id"`
	Status  string `json:"status"`
}

func NewCallbackTask(orderID uint, status string) (*asynq.Task, error) {
	b, err := json.Marshal(CallbackPayload{OrderID: orderID, Status: status})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeCallbackA, b), nil
}

func NewCheckAbandonedTask(orderID uint) (*asynq.Task, error) {
	b, err := json.Marshal(map[string]uint{"order_id": orderID})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeCheckAbandoned, b), nil
}
