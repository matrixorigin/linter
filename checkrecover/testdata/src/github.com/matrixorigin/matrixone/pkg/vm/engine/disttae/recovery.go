package disttae

type Transaction struct{}
type txnTable struct{}
type Engine struct{}

func (*Engine) LoadDatabaseCloneSources() {
	worker := func() { defer func() { _ = recover() }() }
	worker()
	// Callback workers are approved; an unreviewed directly launched literal is not.
	go func() { _ = recover() }() // want "unsafe recover"
}

func (*Transaction) dumpInsertBatchLocked() {
	prepare := func() { defer func() { _ = recover() }() }
	prepare()
	go func() { _ = recover() }() // want "unsafe recover"
}

func (*Transaction) compactDeletionOnObjsLocked() {
	defer func() { _ = recover() }()
	// The synchronous approval must not authorize a directly launched worker.
	go func() { _ = recover() }() // want "unsafe recover"
}
func (*txnTable) rewriteObjectByDeletion()         { defer func() { _ = recover() }() }
func (*txnTable) unrelated()                       { defer func() { _ = recover() }() } // want "unsafe recover"
func (*Transaction) unjoinedWorker()               { go func() { _ = recover() }() }    // want "unsafe recover"
func (*txnTable) rewriteObjectByDeletionInWorker() { go func() { _ = recover() }() }    // want "unsafe recover"
