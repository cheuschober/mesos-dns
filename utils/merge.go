package utils

import (
	"fmt"
	"reflect"

	"github.com/mesosphere/mesos-dns/logging"
)

func Merge(data interface{}, config interface{}) {
	if m, ok := data.(map[string]interface{}); ok {
		for k, v := range m {
			err := SetField(config, k, v)
			// May want to do more than just log these errors
			if err != nil {
				logging.Verbose.Printf("Error merging config key %v:", k, err)
			}
		}
	}
}

// Helper function for merging provided key/value with a struct
func SetField(obj interface{}, name string, value interface{}) error {
	structValue := reflect.ValueOf(obj)
	structFieldValue := reflect.Indirect(structValue).FieldByName(name)

	// Check if field exists
	if !structFieldValue.IsValid() {
		return fmt.Errorf("No such field: %s in obj", name)
	}

	// Check if field is settable
	if !structFieldValue.CanSet() {
		return fmt.Errorf("Cannot set %s field value", name)
	}

	structFieldType := structFieldValue.Type()
	val := reflect.ValueOf(value)
	if structFieldType != val.Type() {
		// JSON converts all numbers to float64
		switch val.Kind().String() {
		case "float64":
			newVal, err := ConvertNum(val.Float(), obj, name)
			if err != nil {
				return err
			}
			structFieldValue.Set(reflect.ValueOf(newVal))
		case "slice":
			// slice is a tricky one. Sticking to float64 and strings for now
			if v, ok := value.([]float64); ok {
				structFieldValue.Set(reflect.ValueOf(v))
			} else if v, ok := value.([]string); ok {
				structFieldValue.Set(reflect.ValueOf(v))
			}
		default:
			return fmt.Errorf("Passed value (type %s) cannot be used for field (type %s)", val.Type(), structFieldType)
		}
	} else {
		structFieldValue.Set(val)
	}

	return nil
}

func ConvertNum(val float64, obj interface{}, name string) (interface{}, error) {
	// This is the same reflection process from SetField()
	structValue := reflect.ValueOf(obj)
	structFieldValue := reflect.Indirect(structValue).FieldByName(name)

	switch i := structFieldValue.Interface().(type) {
	case int:
		return int(val), nil
	case uint8:
		return uint8(val), nil
	case uint16:
		return uint16(val), nil
	case uint32:
		return uint32(val), nil
	case uint64:
		return uint64(val), nil
	case int8:
		return int8(val), nil
	case int16:
		return int16(val), nil
	case int32:
		return int32(val), nil
	case int64:
		return int64(val), nil
	case float32:
		return float32(val), nil
	case float64:
		return float64(val), nil
	default:
		return nil, fmt.Errorf("Unexpected type found for field %s in obj: %s", name, reflect.TypeOf(i))
	}
}
