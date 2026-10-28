package checkunsafe

import (
	"fmt"
	"path"
)

var importWhiteList = []string{
	"github.com/matrixorigin/matrixone/cgo/lib.go",
	"github.com/matrixorigin/matrixone/cmd/mo-service/debug.go",
	"github.com/matrixorigin/matrixone/pkg/cnservice/server_query_test.go",
	"github.com/matrixorigin/matrixone/pkg/common/arenaskl/arena.go",
	"github.com/matrixorigin/matrixone/pkg/common/arenaskl/skl.go",
	"github.com/matrixorigin/matrixone/pkg/common/bitmap/bitmap.go",
	"github.com/matrixorigin/matrixone/pkg/common/bloomfilter/util.go",
	"github.com/matrixorigin/matrixone/pkg/common/buffer/buffer.go",
	"github.com/matrixorigin/matrixone/pkg/common/buffer/chunk.go",
	"github.com/matrixorigin/matrixone/pkg/common/docfilter/filter.go",
	"github.com/matrixorigin/matrixone/pkg/common/fastrand/fastrand.go",
	"github.com/matrixorigin/matrixone/pkg/common/hashmap/inthashmap.go",
	"github.com/matrixorigin/matrixone/pkg/common/hashmap/iterator.go",
	"github.com/matrixorigin/matrixone/pkg/common/hashmap/strhashmap.go",
	"github.com/matrixorigin/matrixone/pkg/common/hashtable/bucket.go",
	"github.com/matrixorigin/matrixone/pkg/common/malloc/checked_allocator.go",
	"github.com/matrixorigin/matrixone/pkg/common/malloc/fixed_size_mmap_allocator.go",
	"github.com/matrixorigin/matrixone/pkg/common/malloc/managed_allocator.go",
	"github.com/matrixorigin/matrixone/pkg/common/malloc/mmap_linux.go",
	"github.com/matrixorigin/matrixone/pkg/common/malloc/profiler.go",
	"github.com/matrixorigin/matrixone/pkg/common/malloc/read_only_allocator.go",
	"github.com/matrixorigin/matrixone/pkg/common/malloc/stacktrace.go",
	"github.com/matrixorigin/matrixone/pkg/common/malloc/unsafe.go",
	"github.com/matrixorigin/matrixone/pkg/common/moprobe/probe.go",
	"github.com/matrixorigin/matrixone/pkg/common/mpool/ptrlen.go",
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
	"github.com/matrixorigin/matrixone/pkg/sql/colexec/aggexec/stateSize.go",
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
	"github.com/matrixorigin/matrixone/pkg/vectorindex/usearchex/search_test.go",
	"github.com/matrixorigin/matrixone/pkg/vm/engine/disttae/cache/types.go",
	"github.com/matrixorigin/matrixone/pkg/vm/engine/disttae/logtailreplay/partition_state.go",
	"github.com/matrixorigin/matrixone/pkg/vm/engine/disttae/logtailreplay/types.go",
	"github.com/matrixorigin/matrixone/pkg/vm/engine/disttae/mo_table_stats.go",
	"github.com/matrixorigin/matrixone/pkg/vm/engine/disttae/txn_table.go",
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
	"github.com/matrixorigin/matrixone/pkg/common/malloc/mmap_darwin.go",
	"github.com/matrixorigin/matrixone/pkg/container/hashtable/hash_arm64.go",
	"github.com/matrixorigin/matrixone/pkg/objectio/meminfo_darwin.go",
	// Existing pointer ownership and physical-layout regression tests.
	"github.com/matrixorigin/matrixone/pkg/common/bitmap/bitmap_test.go",
	"github.com/matrixorigin/matrixone/pkg/common/hashmap/inthashmap_test.go",
	"github.com/matrixorigin/matrixone/pkg/common/hashmap/strhashmap_lazy_test.go",
	"github.com/matrixorigin/matrixone/pkg/common/malloc/mmap_reclaim_test.go",
	"github.com/matrixorigin/matrixone/pkg/common/mpool/allocation_account_mpool_test.go",
	"github.com/matrixorigin/matrixone/pkg/common/mpool/mpool_onheap_ownership_test.go",
	"github.com/matrixorigin/matrixone/pkg/common/mpool/mpool_profile_test.go",
	"github.com/matrixorigin/matrixone/pkg/common/mpool/mpool_test.go",
	"github.com/matrixorigin/matrixone/pkg/common/mpool/realloc_failure_linux_test.go",
	"github.com/matrixorigin/matrixone/pkg/container/arrowbridge/bridge_test.go",
	"github.com/matrixorigin/matrixone/pkg/container/hashtable/allocation_account_test.go",
	"github.com/matrixorigin/matrixone/pkg/container/hashtable/prehashed_lookup_edge_test.go",
	"github.com/matrixorigin/matrixone/pkg/container/hashtable/stable_hash_test.go",
	"github.com/matrixorigin/matrixone/pkg/container/pSpool/sender_test.go",
	"github.com/matrixorigin/matrixone/pkg/container/types/collation_metadata_test.go",
	"github.com/matrixorigin/matrixone/pkg/container/vector/buffer_lease_test.go",
	"github.com/matrixorigin/matrixone/pkg/container/vector/functionTool_test.go",
	"github.com/matrixorigin/matrixone/pkg/lockservice/txn_test.go",
	"github.com/matrixorigin/matrixone/pkg/sql/colexec/external/reader_arrow_test.go",
	"github.com/matrixorigin/matrixone/pkg/sql/colexec/partition/hash_test.go",
	"github.com/matrixorigin/matrixone/pkg/sql/plan/base_binder_case_test.go",
	"github.com/matrixorigin/matrixone/pkg/sql/plan/legacy_tinytext_cache_test.go",
	"github.com/matrixorigin/matrixone/pkg/vectorindex/brute_force/indexsize_test.go",
	"github.com/matrixorigin/matrixone/pkg/vectorindex/hnsw/quantization_test.go",
	"github.com/matrixorigin/matrixone/pkg/vm/engine/tae/db/gc/v3/window_test.go",

	// Physical size/layout accounting at these existing allocation owners.
	"github.com/matrixorigin/matrixone/pkg/common/bloomfilter/types.go",
	"github.com/matrixorigin/matrixone/pkg/container/hashtable/allocation_account.go",
	"github.com/matrixorigin/matrixone/pkg/fulltext2/build_pipeline.go",
	"github.com/matrixorigin/matrixone/pkg/sql/colexec/analytic_allocation.go",
	"github.com/matrixorigin/matrixone/pkg/sql/colexec/dependent/join.go",
	"github.com/matrixorigin/matrixone/pkg/sql/colexec/dependent/kernel.go",
	"github.com/matrixorigin/matrixone/pkg/sql/colexec/dependent/program.go",
	"github.com/matrixorigin/matrixone/pkg/sql/colexec/dependent/recursive.go",
	"github.com/matrixorigin/matrixone/pkg/sql/colexec/dependent/setup.go",
	"github.com/matrixorigin/matrixone/pkg/sql/colexec/dependent/source.go",
	"github.com/matrixorigin/matrixone/pkg/sql/colexec/dependent_expression.go",
	"github.com/matrixorigin/matrixone/pkg/sql/colexec/dependent_reducer.go",
	"github.com/matrixorigin/matrixone/pkg/sql/colexec/mergecte/mergecte.go",
	"github.com/matrixorigin/matrixone/pkg/sql/colexec/mergetop/types.go",
	"github.com/matrixorigin/matrixone/pkg/sql/colexec/order/types.go",
	"github.com/matrixorigin/matrixone/pkg/sql/colexec/partition/hash.go",
	"github.com/matrixorigin/matrixone/pkg/sql/colexec/set_operation_allocation.go",
	"github.com/matrixorigin/matrixone/pkg/sql/colexec/window/window.go",
	"github.com/matrixorigin/matrixone/pkg/sql/compile/dependent.go",
	"github.com/matrixorigin/matrixone/pkg/sql/compile/dependent_external.go",
	"github.com/matrixorigin/matrixone/pkg/sql/plan/function/func_cast_integer_assignment.go",
	"github.com/matrixorigin/matrixone/pkg/vm/continuation.go",
	"github.com/matrixorigin/matrixone/pkg/vm/process/dependent_scope.go",

	// Existing CGo/allocator, zero-copy, alignment, and stable-hash boundaries.
	"github.com/matrixorigin/matrixone/pkg/common/hashmap/keycodec/keycodec.go",
	"github.com/matrixorigin/matrixone/pkg/common/malloc/mmap_reclaim.go",
	"github.com/matrixorigin/matrixone/pkg/container/arrowbridge/convert.go",
	"github.com/matrixorigin/matrixone/pkg/container/hashtable/stable_hash.go",
	"github.com/matrixorigin/matrixone/pkg/sql/colexec/external/arrowio/reader.go",
	"github.com/matrixorigin/matrixone/pkg/sql/colexec/shuffle/shufflepool.go",
	"github.com/matrixorigin/matrixone/pkg/sql/plan/function/byte_like_wildcard_search.go",
	"github.com/matrixorigin/matrixone/pkg/vectorindex/hnsw/types.go",

	// Protobuf-generated descriptor storage; other generated files remain checked.
	"github.com/matrixorigin/matrixone/pkg/datastream/v1/datastream.pb.go",
}

// CGo approvals name the authored source, not its generated Go output.
var cgoWhiteList = []string{
	"github.com/matrixorigin/matrixone/cgo/lib.go",
	"github.com/matrixorigin/matrixone/pkg/common/bitmap/cbitmap.go",
	"github.com/matrixorigin/matrixone/pkg/common/bloomfilter/cbloomfilter.go",
	"github.com/matrixorigin/matrixone/pkg/common/docfilter/cbitmap.go",
	"github.com/matrixorigin/matrixone/pkg/common/docfilter/croaring.go",
	"github.com/matrixorigin/matrixone/pkg/common/malloc/c_allocator.go",
	"github.com/matrixorigin/matrixone/pkg/common/malloc/c_allocator_linux.go",
	"github.com/matrixorigin/matrixone/pkg/common/malloc/fixed_size_mimalloc_allocator.go",
	"github.com/matrixorigin/matrixone/pkg/common/malloc/sane_allocator.go",
	"github.com/matrixorigin/matrixone/pkg/common/malloc/sane_allocator_linux.go",
	"github.com/matrixorigin/matrixone/pkg/sql/plan/function/cxcall.go",
	"github.com/matrixorigin/matrixone/pkg/vectorindex/usearchex/search.go",
	"github.com/matrixorigin/matrixone/pkg/common/docfilter/cmembership.go",
	"github.com/matrixorigin/matrixone/pkg/common/docfilter/sorted64.go",
	"github.com/matrixorigin/matrixone/pkg/common/malloc/jemalloc_allocator.go",
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

// Message checks an authored unsafe import using its logical source and provenance.
func Message(pkgPath, fileName string, cgo bool) string {
	key := path.Join(pkgPath, fileName)
	if cgo {
		for _, approved := range cgoWhiteList {
			if approved == key {
				return ""
			}
		}
	} else if importOK(key) {
		return ""
	}
	return fmt.Sprintf("importing the unsafe package is not allowed in this file: %s", key)
}
