package orm

import "context"

// exec 将中间件链起来
func exec(ctx context.Context, core core, session Session, qc *QueryContext) *QueryResult {
	var root Handler = func(ctx context.Context, queryCtx *QueryContext) *QueryResult {
		return execHandler(ctx, session, queryCtx)
	}
	for i := len(core.middlewares) - 1; i >= 0; i-- {
		root = core.middlewares[i](root)
	}

	return root(ctx, qc)
}

func execHandler(ctx context.Context, session Session, queryCtx *QueryContext) *QueryResult {
	query, err := queryCtx.QueryBuilder.Build()
	if err != nil {
		return &QueryResult{Error: err, Result: Result{err: err}}
	}

	res, err := session.execContext(ctx, query.SQL, query.Args...)
	return &QueryResult{Result: Result{err: err, res: res}, Error: err}
}
