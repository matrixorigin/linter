package checkunsafe

import (
	"path/filepath"

	"golang.org/x/tools/go/analysis"
)

var Analyzer = &analysis.Analyzer{
	Name: "checkunsafe",
	Doc:  "Tool to check usages of the unsafe package",
	Run:  run,
}

var importWhiteList = []string{
	"github.com/matrixorigin/matrixone/cgo/_cgo_gotypes.go",
	"github.com/matrixorigin/matrixone/cgo/lib.cgo1.go",
	"github.com/matrixorigin/matrixone/cgo/lib.go",
	"github.com/matrixorigin/matrixone/cmd/mo-service/debug.go",
	"github.com/matrixorigin/matrixone/pkg/cnservice/server_query_test.go",
	"github.com/matrixorigin/matrixone/pkg/common/arenaskl/arena.go",
	"github.com/matrixorigin/matrixone/pkg/common/arenaskl/skl.go",
	"github.com/matrixorigin/matrixone/pkg/common/bitmap/_cgo_gotypes.go",
	"github.com/matrixorigin/matrixone/pkg/common/bitmap/bitmap.go",
	"github.com/matrixorigin/matrixone/pkg/common/bitmap/cbitmap.cgo1.go",
	"github.com/matrixorigin/matrixone/pkg/common/bloomfilter/util.go",
	"github.com/matrixorigin/matrixone/pkg/common/buffer/buffer.go",
	"github.com/matrixorigin/matrixone/pkg/common/buffer/chunk.go",
	"github.com/matrixorigin/matrixone/pkg/common/fastrand/fastrand.go",
	"github.com/matrixorigin/matrixone/pkg/common/hashmap/inthashmap.go",
	"github.com/matrixorigin/matrixone/pkg/common/hashmap/iterator.go",
	"github.com/matrixorigin/matrixone/pkg/common/hashmap/strhashmap.go",
	"github.com/matrixorigin/matrixone/pkg/common/hashtable/bucket.go",
	"github.com/matrixorigin/matrixone/pkg/common/malloc/_cgo_gotypes.go",
	"github.com/matrixorigin/matrixone/pkg/common/malloc/c_allocator.cgo1.go",
	"github.com/matrixorigin/matrixone/pkg/common/malloc/checked_allocator.go",
	"github.com/matrixorigin/matrixone/pkg/common/malloc/fixed_size_mmap_allocator.go",
	"github.com/matrixorigin/matrixone/pkg/common/malloc/managed_allocator.go",
	"github.com/matrixorigin/matrixone/pkg/common/malloc/mmap_linux.go",
	"github.com/matrixorigin/matrixone/pkg/common/malloc/profiler.go",
	"github.com/matrixorigin/matrixone/pkg/common/malloc/read_only_allocator.go",
	"github.com/matrixorigin/matrixone/pkg/common/malloc/stacktrace.go",
	"github.com/matrixorigin/matrixone/pkg/common/malloc/unsafe.go",
	"github.com/matrixorigin/matrixone/pkg/common/moprobe/probe.go",
	"github.com/matrixorigin/matrixone/pkg/common/mpool/alloc.go",
	"github.com/matrixorigin/matrixone/pkg/common/mpool/mpool.go",
	"github.com/matrixorigin/matrixone/pkg/common/reuse/checker.go",
	"github.com/matrixorigin/matrixone/pkg/common/reuse/factory.go",
	"github.com/matrixorigin/matrixone/pkg/common/reuse/mpool_based.go",
	"github.com/matrixorigin/matrixone/pkg/common/system/notify_linux.go",
	"github.com/matrixorigin/matrixone/pkg/common/util/unsafe.go",
	"github.com/matrixorigin/matrixone/pkg/container/hashtable/common.go",
	"github.com/matrixorigin/matrixone/pkg/container/hashtable/hash.go",
	"github.com/matrixorigin/matrixone/pkg/container/hashtable/hash_amd64.go",
	"github.com/matrixorigin/matrixone/pkg/container/hashtable/int64_hash_map.go",
	"github.com/matrixorigin/matrixone/pkg/container/hashtable/string_hash_map.go",
	"github.com/matrixorigin/matrixone/pkg/container/nulls/nulls.go",
	"github.com/matrixorigin/matrixone/pkg/container/types/array_str.go",
	"github.com/matrixorigin/matrixone/pkg/container/types/bytes.go",
	"github.com/matrixorigin/matrixone/pkg/container/types/encoding.go",
	"github.com/matrixorigin/matrixone/pkg/container/types/packer.go",
	"github.com/matrixorigin/matrixone/pkg/container/types/rowid.go",
	"github.com/matrixorigin/matrixone/pkg/container/types/rowid_test.go",
	"github.com/matrixorigin/matrixone/pkg/container/types/timestamp.go",
	"github.com/matrixorigin/matrixone/pkg/container/types/timestamp_test.go",
	"github.com/matrixorigin/matrixone/pkg/container/types/tuple.go",
	"github.com/matrixorigin/matrixone/pkg/container/types/txnts.go",
	"github.com/matrixorigin/matrixone/pkg/container/vector/tools.go",
	"github.com/matrixorigin/matrixone/pkg/container/vector/vector.go",
	"github.com/matrixorigin/matrixone/pkg/fileservice/fifocache/data_cache.go",
	"github.com/matrixorigin/matrixone/pkg/fileservice/fifocache/shard.go",
	"github.com/matrixorigin/matrixone/pkg/fileservice/pool.go",
	"github.com/matrixorigin/matrixone/pkg/frontend/util.go",
	"github.com/matrixorigin/matrixone/pkg/objectio/block_info.go",
	"github.com/matrixorigin/matrixone/pkg/objectio/codecs.go",
	"github.com/matrixorigin/matrixone/pkg/objectio/id.go",
	"github.com/matrixorigin/matrixone/pkg/objectio/id_test.go",
	"github.com/matrixorigin/matrixone/pkg/objectio/location.go",
	"github.com/matrixorigin/matrixone/pkg/objectio/meta.go",
	"github.com/matrixorigin/matrixone/pkg/objectio/name.go",
	"github.com/matrixorigin/matrixone/pkg/objectio/utils.go",
	"github.com/matrixorigin/matrixone/pkg/sql/compile/remoterun.go",
	"github.com/matrixorigin/matrixone/pkg/sql/parsers/dialect/mysql/mysql_sql.go",
	"github.com/matrixorigin/matrixone/pkg/sql/parsers/dialect/postgresql/postgresql_sql.go",
	"github.com/matrixorigin/matrixone/pkg/sql/plan/function/func_builtin.go",
	"github.com/matrixorigin/matrixone/pkg/sql/plan/function/func_cast.go",
	"github.com/matrixorigin/matrixone/pkg/sql/plan/function/func_unary.go",
	"github.com/matrixorigin/matrixone/pkg/sql/plan/shuffle.go",
	"github.com/matrixorigin/matrixone/pkg/sql/plan/shuffle_test.go",
	"github.com/matrixorigin/matrixone/pkg/testutil/util_new.go",
	"github.com/matrixorigin/matrixone/pkg/util/core_dump.go",
	"github.com/matrixorigin/matrixone/pkg/util/rand.go",
	"github.com/matrixorigin/matrixone/pkg/util/trace/impl/motrace/mo_trace.go",
	"github.com/matrixorigin/matrixone/pkg/util/trace/impl/motrace/report_error.go",
	"github.com/matrixorigin/matrixone/pkg/util/trace/impl/motrace/report_log.go",
	"github.com/matrixorigin/matrixone/pkg/util/trace/impl/motrace/report_statement.go",
	"github.com/matrixorigin/matrixone/pkg/vm/engine/disttae/cache/types.go",
	"github.com/matrixorigin/matrixone/pkg/vm/engine/disttae/logtailreplay/partition_state.go",
	"github.com/matrixorigin/matrixone/pkg/vm/engine/disttae/logtailreplay/types.go",
	"github.com/matrixorigin/matrixone/pkg/vm/engine/disttae/txn_table.go",
	"github.com/matrixorigin/matrixone/pkg/vm/engine/disttae/mo_table_stats.go",
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/catalog/basemvccnode.go",
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/catalog/database.go",
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/catalog/metamvccnode.go",
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/common/id.go",
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/common/interval.go",
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/compute/compare.go",
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/containers/batch.go",
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/containers/pool.go",
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/containers/runtime_1.22.go",
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/db/gc/v3/exec_v1.go",
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/db/test/storage_usage_test.go",
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/index/hybrid_filter.go",
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/index/zm.go",
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/logstore/entry/desc.go",
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/logtail/service/session.go",
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/logtail/storage_usage.go",
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/model/tree.go",
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/tables/updates/mvcc.go",
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/txn/txnimpl/basenode.go",
}

var importOK = func() func(path string) bool {
	m := make(map[string]bool)
	for _, path := range importWhiteList {
		m[path] = true
	}
	return func(path string) bool {
		return m[path]
	}
}()

func run(pass *analysis.Pass) (interface{}, error) {

	// check imports in files
	for _, file := range pass.Files {
		pos := pass.Fset.Position(file.FileStart)
		fileName := filepath.Base(pos.Filename)
		for _, imp := range file.Imports {
			path := imp.Path.Value
			if path != `"unsafe"` {
				continue
			}
			specPath := filepath.Join(pass.Pkg.Path(), fileName)
			if !importOK(specPath) {
				pass.Reportf(
					imp.Pos(),
					"importing the unsafe package is not allowed in this file: %v",
					specPath,
				)
			}
		}
	}

	return nil, nil
}
