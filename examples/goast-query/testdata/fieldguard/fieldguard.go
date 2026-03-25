package fieldguard

import "log"

type Request struct {
	Valid bool
	Data  string
}

func HandleSafeA(r Request) string {
	if !r.Valid {
		return "invalid"
	}
	return r.Data
}

func HandleSafeB(r Request) string {
	if !r.Valid {
		log.Println("invalid request")
		return ""
	}
	return r.Data
}

func HandleSafeC(r Request) string {
	if !r.Valid {
		return "bad"
	}
	return r.Data
}

func HandleSafeD(r Request) string {
	if !r.Valid {
		return "err"
	}
	return r.Data
}

// HandleUnsafe uses r without checking Valid — intentional deviation.
func HandleUnsafe(r Request) string {
	log.Println("data:", r.Data)
	return r.Data
}
