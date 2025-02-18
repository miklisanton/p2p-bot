package utils

import (
	"fmt"
	"p2pbot/internal/db/models"
	"reflect"

	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
)

func GetField(obj interface{}, name string) (interface{}, error) {
	v := reflect.ValueOf(obj).Elem()

	field := v.FieldByName(name)
	if !field.IsValid() {
		return nil, fmt.Errorf("no such field: %s in obj", name)
	}

	return field.Interface(), nil
}

func SetField(obj interface{}, name string, value interface{}) error {
	v := reflect.ValueOf(obj).Elem()

	field := v.FieldByName(name)
	if !field.IsValid() {
		return fmt.Errorf("no such field: %s in obj", name)
	}
	if !field.CanSet() {
		return fmt.Errorf("cannot set %s field value", name)
	}

	field.Set(reflect.ValueOf(value))
	return nil
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func Contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

// Returns true if one of trackers payment methods is in ads payment methods
func ComparePaymentMethods(adMethods []string, trackerMethods []*models.PaymentMethod) bool {
	for _, trackerMethod := range trackerMethods {
		if Contains(adMethods, trackerMethod.Id) {
			return true
		}
	}
	return false
}

func AllOutbidded(pMethods []*models.PaymentMethod) bool {
	for _, pMethod := range pMethods {
		if !pMethod.Outbided {
			return false
		}
	}
	return true
}

// Returns email and chat_id from echo.Context
// if email is present, chat_id is -1
// if chat_id is present, email is ""
// if none or both are present, error is returned
func RetreiveEmailNChatID(ctx echo.Context) (string, int64, error) {
	email := ctx.Get("email")
	chatID := ctx.Get("chat_id")
	if email != nil && chatID == nil {
		return email.(string), -1, nil
	}
	if email == nil && chatID != nil {
		return "", chatID.(int64), nil
	}
	return "", -1, fmt.Errorf("email xor chat_id must be provided in context")
}
