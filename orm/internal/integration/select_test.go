package integration

import (
	"github.com/stretchr/testify/suite"
	"testing"
)

type SelectSuite struct {
	suite.Suite
	driver string
	dsn    string
}

func (s *SelectSuite) TestSelect(t *testing.T) {
	suite.Run(t, &SelectSuite{
		driver: "mysql",
		dsn:    "root:root@tcp(localhost:13306)/integration_test?charset=utf8",
	})
}
