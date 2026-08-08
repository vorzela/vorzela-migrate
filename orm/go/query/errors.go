package query

import "errors"

// ErrGeneratePending is returned for stubs vorm generate could not fully lower yet
// (Find/Paginate/Transaction, dynamic Limit, …). Prefer gen.* for lowered queries.
var ErrGeneratePending = errors.New("vorm: query not fully generated — simplify stub or use models.* builders")
