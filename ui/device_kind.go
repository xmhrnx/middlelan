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
		return tr("Router")
	case "phone":
		return tr("Phone or tablet")
	case "computer":
		return tr("Computer")
	case "printer":
		return tr("Printer")
	case "tv":
		return tr("TV or streamer")
	case "camera":
		return tr("Camera")
	case "speaker":
		return tr("Speaker")
	case "iot":
		return tr("Smart device")
	}
	return tr("Unknown device")
}
