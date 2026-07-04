package orm

import "database/sql"

type Result struct {
	err error
	res sql.Result
}

// Err 返回执行过程中发生的错误（若有）。
// 调用方可使用 errors.Is(err, errs.ErrXxx) 对 sentinel 错误进行匹配。
func (r Result) Err() error {
	return r.err
}

func (r Result) RowsAffected() (int64, error) {
	if r.err != nil {
		return 0, r.err
	}
	return r.res.RowsAffected()
}

func (r Result) LastInsertId() (int64, error) {
	if r.err != nil {
		return 0, r.err
	}
	return r.res.LastInsertId()
}
