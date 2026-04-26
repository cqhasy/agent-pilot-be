package state

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func Generate(returnTo string, secret string) (string, error) {
	data := fmt.Sprintf("%d|%s", time.Now().Unix(), returnTo)

	dataEncoded := base64.URLEncoding.EncodeToString([]byte(data))

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(dataEncoded))
	signature := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	return dataEncoded + "." + signature, nil
}

func Verify(state string, secret string, expireSeconds int64) (string, error) {
	parts := strings.Split(state, ".")
	if len(parts) != 2 {
		return "", errors.New("invalid state format")
	}

	dataEncoded := parts[0]
	signature := parts[1]

	// 校验签名
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(dataEncoded))
	expected := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return "", errors.New("state tampered")
	}

	// 解码
	dataBytes, err := base64.URLEncoding.DecodeString(dataEncoded)
	if err != nil {
		return "", err
	}

	arr := strings.SplitN(string(dataBytes), "|", 2)
	if len(arr) != 2 {
		return "", errors.New("invalid state data")
	}

	// 校验过期
	ts, _ := strconv.ParseInt(arr[0], 10, 64)
	if time.Now().Unix()-ts > expireSeconds {
		return "", errors.New("state expired")
	}

	returnTo := arr[1]
	return returnTo, nil
}
