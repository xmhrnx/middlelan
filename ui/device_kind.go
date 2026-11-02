package ui

// deviceIcon maps a device kind to its icon.
func deviceIcon(kind string) string {
	switch kind {
	case "router":
		return "router"
	case "phone":
		return "smartphone"
	case "computer":
		return "laptop"
	case "printer":
		return "printer"
	case "tv":
		return "tv"
	case "camera":
		return "camera"
	case "speaker":
		return "speaker"
	case "iot":
		return "cpu"
	}
	return "circle-question-mark"
}

// deviceKindLabel names a device kind for people.
func deviceKindLabel(kind string) string {
	switch kind {
	case "router":
		return "Router"
	case "phone":
		return "Phone or tablet"
	case "computer":
		return "Computer"
	case "printer":
		return "Printer"
	case "tv":
		return "TV or streamer"
	case "camera":
		return "Camera"
	case "speaker":
		return "Speaker"
	case "iot":
		return "Smart device"
	}
	return "Unknown device"
}
