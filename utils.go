package easyws

func IntValue(a interface{}) (int, bool) {
	if a == nil {
		return 0, false
	}
	switch a.(type) {
	case int:
		return a.(int), true
	case int8:
		return int(a.(int8)), true
	case int16:
		return int(a.(int16)), true
	case int32:
		return int(a.(int32)), true
	case uint:
		return int(a.(uint)), true
	case uint8:
		return int(a.(uint8)), true
	case uint16:
		return int(a.(uint16)), true
	case uint32:
		return int(a.(uint32)), true
	case uint64:
		return int(a.(uint64)), true
	case float32:
		return int(a.(float32)), true
	case float64:
		return int(a.(float64)), true
	default:
		return 0, false
	}
}
