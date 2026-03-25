package checking

import "log"

func HandleSafeA(err error) string {
	if err != nil {
		log.Println("error:", err)
		return "error"
	}
	return "ok"
}

func HandleSafeB(err error) string {
	if err != nil {
		return err.Error()
	}
	return "ok"
}

func HandleSafeC(err error) string {
	if err != nil {
		log.Println(err)
		return "failed"
	}
	return "ok"
}

func HandleSafeD(err error) string {
	if err != nil {
		return "err: " + err.Error()
	}
	return "ok"
}

// HandleUnsafe uses err without checking — intentional deviation.
func HandleUnsafe(err error) string {
	log.Println("processing:", err)
	return "done"
}
