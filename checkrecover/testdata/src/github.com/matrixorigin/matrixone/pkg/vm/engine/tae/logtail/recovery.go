package logtail

func ReWriteCheckpointAndBlockFromKey() { defer func() { _ = recover() }() }
func unrelatedBackup()                  { defer func() { _ = recover() }() } // want "unsafe recover"
