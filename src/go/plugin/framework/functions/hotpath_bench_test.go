package functions

import (
	"context"
	"strings"
	"testing"
)

func BenchmarkBFunctionIngress(b *testing.B) {
	const input = "" +
		"FUNCTION uid 30 \"module:method arg\" 0xFFFF \"method=api\"\n" +
		"QUIT\n"
	b.ReportAllocs()
	for b.Loop() {
		capsule, err := NewInputCapsule(
			strings.NewReader(input),
			&immediateCapsuleBodyBudget{},
		)
		if err != nil {
			b.Fatal(err)
		}
		consumer := &recordingCapsuleConsumer{}
		if err := capsule.Run(
			context.Background(),
			consumer,
		); err != nil {
			b.Fatal(err)
		}
		if len(consumer.calls) != 1 || !consumer.quit {
			b.Fatal("Function ingress did not complete")
		}
	}
}
