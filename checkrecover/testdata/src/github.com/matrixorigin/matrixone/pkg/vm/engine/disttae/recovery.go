package disttae

type Transaction struct{}
type txnTable struct{}

func (*Transaction) compactDeletionOnObjsLocked()  { defer func() { _ = recover() }() }
func (*txnTable) rewriteObjectByDeletion()         { defer func() { _ = recover() }() }
func (*txnTable) unrelated()                       { defer func() { _ = recover() }() } // want "unsafe recover"
func (*Transaction) unjoinedWorker()               { go func() { _ = recover() }() }    // want "unsafe recover"
func (*txnTable) rewriteObjectByDeletionInWorker() { go func() { _ = recover() }() }    // want "unsafe recover"
