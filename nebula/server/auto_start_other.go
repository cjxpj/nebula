//go:build !windows && !linux && !darwin

package dic_server

import "errors"

func SetAutoStart() error {
	return errors.New("开机自启不支持当前系统")
}

func CancelAutoStart() error {
	return errors.New("开机自启不支持当前系统")
}

func GetAutoStart() (bool, error) {
	return false, nil
}
