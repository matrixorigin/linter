# MatrixOrigin Linters

This repo contains custom linters developed by MatrixOrigin.

## recover() linter

As required by our error handling rules, "You should not recover any panics unless it is at a few well defined places". This linter helps to enforce that. 

Install the linter - 
```
go install github.com/matrixorigin/linter/tools/recoverlinter
```

Make sure your system can find the tool - 
```
lni@youdian:~/src/matrixone$ which recoverlinter
/home/lni/go/bin/recoverlinter
```

The following command should pass as all calls to recover() are marked as approved.  

```
cd ~/src/matrixone
go vet -vettool=`which recoverlinter` ./pkg/sql/compile
```

It will fail when checking the frontend folder as recover() calls there are not currently marked as approved. 

```
cd ~/src/matrixone
go vet -vettool=`which recoverlinter` ./pkg/frontend

# github.com/matrixorigin/matrixone/pkg/frontend
pkg/frontend/load.go:1491:12: unsafe recover() found: in github.com/matrixorigin/matrixone/pkg/frontend.MysqlCmdExecutor.LoadLoop()
pkg/frontend/routine\_manager.go:45:13: unsafe recover() found: in github.com/matrixorigin/matrixone/pkg/frontend.RoutineManager.Created()
pkg/frontend/routine\_manager.go:73:13: unsafe recover() found: in github.com/matrixorigin/matrixone/pkg/frontend.RoutineManager.Closed()
pkg/frontend/routine\_manager.go:113:13: unsafe recover() found: in github.com/matrixorigin/matrixone/pkg/frontend.RoutineManager.Handler()
```
