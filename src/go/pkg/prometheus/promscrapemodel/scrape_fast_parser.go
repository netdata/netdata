// SPDX-License-Identifier: GPL-3.0-or-later

package promscrapemodel

import "github.com/netdata/netdata/go/plugins/pkg/prometheus/promselector"

type scrapeFastParser struct {
	driver parseDriver
}

type FastParser struct {
	parser scrapeFastParser
}

func NewFastParser(sr promselector.Selector) FastParser {
	return FastParser{parser: scrapeFastParser{driver: parseDriver{sr: sr}}}
}

func (p *FastParser) ParseToAssembler(text []byte, asm *Assembler) error {
	return p.parser.parseToAssembler(text, asm)
}

func (p *scrapeFastParser) parseToAssembler(text []byte, asm *Assembler) error {
	if asm == nil {
		return p.driver.parse(text, false, nil, func(Sample) error { return nil })
	}
	return p.driver.parse(text, false, asm.applyHelp, asm.ApplySample)
}
