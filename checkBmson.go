package checkbms

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"

	"github.com/Shimi9999/checkbms/bmson"
)

func ScanBmson(bytes []byte) (*bmson.Bmson, error) {
	logs := Logs{}

	var bmsonObj interface{}
	if err := json.Unmarshal(bytes, &bmsonObj); err != nil {
		logs = append(logs, Log{
			Level:      Error,
			Message:    fmt.Sprintf("bmson format is invalid: %s", err.Error()),
			Message_ja: fmt.Sprintf("bmsonのフォーマットが無効です: %s", err.Error()),
		})
		for _, log := range logs {
			fmt.Println(log.String())
		}
		return nil, nil
	}

	invalidKeyLog := func(key string, value interface{}, locationKey string) {
		if str, isString := value.(string); isString {
			value = fmt.Sprintf("\"%s\"", str)
		}
		logs = append(logs, Log{
			Level:      Warning,
			Message:    fmt.Sprintf("Invalid key: {\"%s\": %v} in %s", key, value, locationKey),
			Message_ja: fmt.Sprintf("このキーは無効です: {\"%s\": %v} in %s", key, value, locationKey),
		})
	}

	// 型エラー 想定する型と実際の型も表示する？
	invalidTypeLog := func(key string, value interface{}) {
		logs = append(logs, Log{
			Level:      Error,
			Message:    fmt.Sprintf("%s has invalid value: %v", key, value),
			Message_ja: fmt.Sprintf("%sが無効な値です: %s", key, value),
		})
	}

	// bmsonのフォーマットチェックをしながら、bmsonデータを読み込む
	// TODO unmershallのときにmapにまとめられてしまうので、キーの重複が検知できない
	var load func(jsonObj interface{}, dataType reflect.Type, keyName string) reflect.Value
	load = func(jsonObj interface{}, dataType reflect.Type, keyName string) reflect.Value {
		if dataType.Kind() == reflect.Invalid {
			return reflect.Value{}
		}

		isPtr := false
		if dataType.Kind() == reflect.Ptr {
			dataType = dataType.Elem()
			isPtr = true
		}

		switch dataType.Kind() {
		case reflect.Struct:
			if mapVals, isMap := jsonObj.(map[string]interface{}); isMap {
				structData := reflect.New(dataType).Elem()
				dataType := structData.Type()
				for key, val := range mapVals {
					match := false
					for i := 0; i < dataType.NumField(); i++ {
						fieldName := strings.ToLower(dataType.Field(i).Name)
						if key == fieldName {
							match = true
							loadedValue := load(val, dataType.Field(i).Type, fmt.Sprintf("%s.%s", keyName, key))
							if loadedValue.Kind() == reflect.Ptr && dataType.Field(i).Type.Kind() != reflect.Ptr {
								loadedValue = loadedValue.Elem()
							}
							structData.Field(i).Set(loadedValue)
							break
						}
					}
					if !match {
						invalidKeyLog(key, val, keyName)
					}
				}
				if isPtr && structData.Kind() != reflect.Ptr {
					return structData.Addr()
				}
				return structData
			} else {
				invalidTypeLog(keyName, jsonObj)
			}
		case reflect.Slice:
			if sliceVals, isSlice := jsonObj.([]interface{}); isSlice {
				sliceData := reflect.MakeSlice(dataType, len(sliceVals), len(sliceVals))
				for i, val := range sliceVals {
					loadedValue := load(val, dataType.Elem(), fmt.Sprintf("%s[%d]", keyName, i))
					if loadedValue.Kind() == reflect.Ptr && dataType.Elem().Kind() != reflect.Ptr {
						sliceData.Index(i).Set(loadedValue.Elem())
					} else {
						sliceData.Index(i).Set(loadedValue)
					}
				}
				if isPtr && sliceData.Kind() != reflect.Ptr {
					return sliceData.Addr()
				}
				return sliceData
			} else {
				invalidTypeLog(keyName, jsonObj)
			}
		case reflect.String:
			if val, ok := jsonObj.(string); ok {
				return reflect.ValueOf(&val)
			} else {
				invalidTypeLog(keyName, jsonObj)
			}
		case reflect.Int:
			if val, ok := jsonObj.(float64); ok {
				if val-math.Floor(val) != 0 {
					invalidTypeLog(keyName, jsonObj)
				}
				iVal := int(val)
				return reflect.ValueOf(&iVal)
			} else {
				invalidTypeLog(keyName, jsonObj)
			}
		case reflect.Float64:
			if val, ok := jsonObj.(float64); ok {
				return reflect.ValueOf(&val)
			} else {
				invalidTypeLog(keyName, jsonObj)
			}
		case reflect.Bool:
			if val, ok := jsonObj.(bool); ok {
				return reflect.ValueOf(&val)
			} else {
				invalidTypeLog(keyName, jsonObj)
			}
		case reflect.Interface:
			return reflect.ValueOf(&jsonObj)
		}

		return reflect.Value{}
	}

	bmsonData := &bmson.Bmson{}
	bmsonData = load(bmsonObj, reflect.TypeOf(bmsonData), "bmson").Interface().(*bmson.Bmson)
	//fmt.Printf("bmsonData: %+v\n", bmsonData)
	/*fmt.Printf("bmsonDataInfo: %+v\n", bmsonData.Info)

	for _, log := range logs {
		fmt.Println(log.String())
	}*/

	return nil, nil
}
