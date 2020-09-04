// -*- Mode: Go; indent-tabs-mode: t -*-
//
// Copyright (C) 2018-2019 IOTech Ltd
//
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"encoding/json"
	"fmt"
	"github.com/edgexfoundry/go-mod-core-contracts/models"
	"net/url"
	"strings"
	"time"

	"github.com/eclipse/paho.mqtt.golang"
	sdkModel "github.com/edgexfoundry/device-sdk-go/pkg/models"
)

func startRegisterListening() error {
	var scheme = driver.Config.RegisterSchema
	var brokerUrl = driver.Config.RegisterHost
	var brokerPort = driver.Config.RegisterPort
	var username = driver.Config.RegisterUser
	var password = driver.Config.RegisterPassword
	var mqttClientId = driver.Config.RegisterClientId
	var qos = byte(driver.Config.RegisterQos)
	var keepAlive = driver.Config.RegisterKeepAlive
	var topic = driver.Config.RegisterTopic

	uri := &url.URL{
		Scheme: strings.ToLower(scheme),
		Host:   fmt.Sprintf("%s:%d", brokerUrl, brokerPort),
		User:   url.UserPassword(username, password),
	}

	var client mqtt.Client
	var err error
	for i := 1; i <= driver.Config.ConnEstablishingRetry; i++ {
		client, err = createClient(mqttClientId, uri, keepAlive)
		if err != nil && i == driver.Config.ConnEstablishingRetry {
			return err
		} else if err != nil {
			driver.Logger.Error(fmt.Sprintf("Fail to initial conn for device register, %v ", err))
			time.Sleep(time.Duration(driver.Config.ConnEstablishingRetry) * time.Second)
			driver.Logger.Warn("Retry to initial conn for device register")
			continue
		}
		break
	}

	defer func() {
		if client.IsConnected() {
			client.Disconnect(5000)
		}
	}()

	token := client.Subscribe(topic, qos, onRegisterReceived)
	if token.Wait() && token.Error() != nil {
		driver.Logger.Info(fmt.Sprintf("[Register listener] Stop device register listening. Cause:%v", token.Error()))
		return token.Error()
	}

	driver.Logger.Info("[Register listener] Start device register listening. ")
	select {}
}

func onRegisterReceived(client mqtt.Client, message mqtt.Message) {
	var register map[string]interface{}
	err := json.Unmarshal(message.Payload(), &register)
	if err != nil {
		return
	}

	if !checkRegisterWithKey(register, "name") || !checkRegisterWithKey(register, "description") {
		return
	}

	deviceName := register["name"].(string)
	description := register["description"].(string)

	protocolsMap, ok := register["protocols"]
	if !ok {
		driver.Logger.Warn(fmt.Sprintf("[Register listener] Register ignored. No %v found : msg=%v",
			"protocols", register))
		return
	}

	protocolMap := protocolsMap.(map[string]interface{})[Protocol]
	protocol := make(map[string]string)
	for key, value := range protocolMap.(map[string]interface{}) {
		protocol[key] = value.(string)
		//driver.Logger.Info(fmt.Sprintf("key: %v, value: %v", key, value))
	}

	protocols := make(map[string]models.ProtocolProperties)
	protocols[Protocol] = protocol

	labelArr, ok := register["labels"].([]interface{})
	if !ok {
		driver.Logger.Warn(fmt.Sprintf("[Register listener] Register ignored. No %v found : msg=%v",
			"labels", register))
		return
	}
	labels := make([]string, len(labelArr))
	for i, v := range labelArr {
		labels[i] = v.(string)
	}

	discoveredDevice := []sdkModel.DiscoveredDevice{
		{
			Name:        deviceName,
			Protocols:   protocols,
			Description: description,
			Labels:      labels,
		},
	}

	driver.Logger.Info(fmt.Sprintf("discoveredDevice: %v", discoveredDevice[0].Name))

	driver.DeviceCh <- discoveredDevice
}

func checkRegisterWithKey(data map[string]interface{}, key string) bool {
	val, ok := data[key]
	if !ok {
		driver.Logger.Warn(fmt.Sprintf("[Register listener] Register ignored. No %v found : msg=%v", key, data))
		return false
	}

	switch val.(type) {
	case string:
		return true
	default:
		driver.Logger.Warn(fmt.Sprintf("[Register listener] Register ignored. %v should be string : msg=%v", key, data))
		return false
	}
}
