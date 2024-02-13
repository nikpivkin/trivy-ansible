package main

import "gopkg.in/yaml.v3"

type Range struct {
	startLine int
	endLine   int
}

// TODO; use meta from Trivy
type Metadata struct {
	path   string
	rng    Range
	parent *Metadata
}

func RangeFromNode(node *yaml.Node) Range {
	return Range{
		startLine: node.Line,
		endLine:   calculateEndLine(node),
	}
}

func calculateEndLine(node *yaml.Node) int {
	for node.Content != nil {
		node = node.Content[len(node.Content)-1]
	}
	return node.Line
}
