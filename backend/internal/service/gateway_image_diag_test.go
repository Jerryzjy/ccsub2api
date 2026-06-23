//go:build unit

package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiagnoseMissingImageSource_ToolResultMissingSource(t *testing.T) {
	body := []byte(`{"messages":[
		{"role":"user","content":[
			{"type":"tool_result","tool_use_id":"t1","content":[
				{"type":"image","url":"https://example.com/x.png"}
			]}
		]}
	]}`)
	out := diagnoseMissingImageSource(body)
	require.Len(t, out, 1)
	require.Contains(t, out[0], "msg[0]")
	require.Contains(t, out[0], "image")
	require.Contains(t, out[0], "url")
}

func TestDiagnoseMissingImageSource_ValidSourceNotReported(t *testing.T) {
	body := []byte(`{"messages":[
		{"role":"user","content":[
			{"type":"tool_result","content":[
				{"type":"image","source":{"type":"base64","media_type":"image/png","data":"abc"}}
			]}
		]}
	]}`)
	require.Empty(t, diagnoseMissingImageSource(body))
}

func TestDiagnoseMissingImageSource_TopLevelImageMissingSource(t *testing.T) {
	body := []byte(`{"messages":[
		{"role":"user","content":[
			{"type":"image","data":"rawbytes"}
		]}
	]}`)
	out := diagnoseMissingImageSource(body)
	require.Len(t, out, 1)
	require.Contains(t, out[0], "msg[0]")
}

func TestDiagnoseMissingImageSource_TruncatesLongValue(t *testing.T) {
	long := strings.Repeat("A", 500)
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"image","url":"` + long + `"}]}]}`)
	out := diagnoseMissingImageSource(body)
	require.Len(t, out, 1)
	require.NotContains(t, out[0], long) // 长值必须被截断
	require.Less(t, len(out[0]), 300)
}

func TestDiagnoseMissingImageSource_NoImageReturnsEmpty(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"hello"}]}`)
	require.Empty(t, diagnoseMissingImageSource(body))
}

func TestIsMissingImageSourceError(t *testing.T) {
	require.True(t, isMissingImageSourceError("messages.22.content.0.tool_result.content.0.image.source: Field required"))
	require.True(t, isMissingImageSourceError("IMAGE.SOURCE: field REQUIRED"))
	require.False(t, isMissingImageSourceError("thinking.signature: Field required"))
	require.False(t, isMissingImageSourceError("prompt is too long"))
}
