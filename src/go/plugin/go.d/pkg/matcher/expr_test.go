// SPDX-License-Identifier: GPL-3.0-or-later

package matcher

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSimpleExpr_none(t *testing.T) {
	expr := &SimpleExpr{}

	m, err := expr.Parse()
	assert.EqualError(t, err, ErrEmptyExpr.Error())
	assert.Nil(t, m)
}

func TestSimpleExpr_include(t *testing.T) {
	expr := &SimpleExpr{
		Includes: []string{
			"~ /api/",
			"~ .php$",
		},
	}

	m, err := expr.Parse()
	assert.NoError(t, err)

	assert.True(t, m.MatchString("/api/a.php"))
	assert.True(t, m.MatchString("/api/a.php2"))
	assert.True(t, m.MatchString("/api2/a.php"))
	assert.True(t, m.MatchString("/api/img.php"))
	assert.False(t, m.MatchString("/api2/img.php2"))
}

func TestSimpleExpr_exclude(t *testing.T) {
	expr := &SimpleExpr{
		Excludes: []string{
			"~ /api/img",
		},
	}

	m, err := expr.Parse()
	assert.NoError(t, err)

	assert.True(t, m.MatchString("/api/a.php"))
	assert.True(t, m.MatchString("/api/a.php2"))
	assert.True(t, m.MatchString("/api2/a.php"))
	assert.False(t, m.MatchString("/api/img.php"))
	assert.True(t, m.MatchString("/api2/img.php2"))
}

func TestSimpleExpr_both(t *testing.T) {
	expr := &SimpleExpr{
		Includes: []string{
			"~ /api/",
			"~ .php$",
		},
		Excludes: []string{
			"~ /api/img",
		},
	}

	m, err := expr.Parse()
	assert.NoError(t, err)

	assert.True(t, m.MatchString("/api/a.php"))
	assert.True(t, m.MatchString("/api/a.php2"))
	assert.True(t, m.MatchString("/api2/a.php"))
	assert.False(t, m.MatchString("/api/img.php"))
	assert.False(t, m.MatchString("/api2/img.php2"))
}

func TestSimpleExpr_Parse_NG(t *testing.T) {
	{
		expr := &SimpleExpr{
			Includes: []string{
				"~ (ab",
				"~ .php$",
			},
		}

		m, err := expr.Parse()
		assert.Error(t, err)
		assert.Nil(t, m)
	}
	{
		expr := &SimpleExpr{
			Excludes: []string{
				"~ (ab",
				"~ .php$",
			},
		}

		m, err := expr.Parse()
		assert.Error(t, err)
		assert.Nil(t, m)
	}
}
