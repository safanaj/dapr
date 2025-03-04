/*
Copyright 2021 The Dapr Authors
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/anypb"

	commonv1pb "github.com/dapr/dapr/pkg/proto/common/v1"
	internalv1pb "github.com/dapr/dapr/pkg/proto/internals/v1"
)

func TestInvocationResponse(t *testing.T) {
	resp := NewInvokeMethodResponse(0, "OK", nil)
	defer resp.Close()

	assert.Equal(t, int32(0), resp.r.GetStatus().Code)
	assert.Equal(t, "OK", resp.r.GetStatus().Message)
	assert.NotNil(t, resp.r.Message)
}

func TestInternalInvocationResponse(t *testing.T) {
	t.Run("valid internal invoke response with no data", func(t *testing.T) {
		m := &commonv1pb.InvokeResponse{
			Data:        nil,
			ContentType: "application/json",
		}
		pb := internalv1pb.InternalInvokeResponse{
			Status:  &internalv1pb.Status{Code: 0},
			Message: m,
		}

		ir, err := InternalInvokeResponse(&pb)
		assert.NoError(t, err)
		defer ir.Close()
		assert.NotNil(t, ir.r.Message)
		assert.Equal(t, int32(0), ir.r.Status.Code)
		assert.Nil(t, ir.r.Message.Data)

		bData, err := io.ReadAll(ir.RawData())
		assert.NoError(t, err)
		assert.Len(t, bData, 0)
	})

	t.Run("valid internal invoke response with data", func(t *testing.T) {
		m := &commonv1pb.InvokeResponse{
			Data:        &anypb.Any{Value: []byte("test")},
			ContentType: "application/json",
		}
		pb := internalv1pb.InternalInvokeResponse{
			Status:  &internalv1pb.Status{Code: 0},
			Message: m,
		}

		ir, err := InternalInvokeResponse(&pb)
		assert.NoError(t, err)
		defer ir.Close()
		assert.NotNil(t, ir.r.Message)
		assert.Equal(t, int32(0), ir.r.Status.Code)
		require.NotNil(t, ir.r.Message.Data)
		require.NotNil(t, ir.r.Message.Data.Value)
		assert.Equal(t, []byte("test"), ir.r.Message.Data.Value)

		bData, err := io.ReadAll(ir.RawData())
		assert.NoError(t, err)
		assert.Equal(t, "test", string(bData))
	})

	t.Run("Message is nil", func(t *testing.T) {
		pb := internalv1pb.InternalInvokeResponse{
			Status:  &internalv1pb.Status{Code: 0},
			Message: nil,
		}

		ir, err := InternalInvokeResponse(&pb)
		assert.NoError(t, err)
		defer ir.Close()
		assert.NotNil(t, ir.r.Message)
		assert.Nil(t, ir.r.Message.Data)
	})
}

func TestResponseData(t *testing.T) {
	t.Run("contenttype is set", func(t *testing.T) {
		resp := NewInvokeMethodResponse(0, "OK", nil).
			WithRawDataString("test").
			WithContentType("application/json")
		defer resp.Close()
		bData, err := io.ReadAll(resp.RawData())
		assert.NoError(t, err)
		contentType := resp.r.Message.ContentType
		assert.Equal(t, "application/json", contentType)
		assert.Equal(t, "test", string(bData))
	})

	t.Run("contenttype is unset", func(t *testing.T) {
		resp := NewInvokeMethodResponse(0, "OK", nil).
			WithRawDataString("test")
		defer resp.Close()

		contentType := resp.ContentType()
		bData, err := io.ReadAll(resp.RawData())
		assert.NoError(t, err)
		assert.Equal(t, "", resp.r.Message.ContentType)
		assert.Equal(t, "", contentType)
		assert.Equal(t, "test", string(bData))
	})

	t.Run("typeurl is set but content_type is unset", func(t *testing.T) {
		s := &commonv1pb.StateItem{Key: "custom_key"}
		b, err := anypb.New(s)
		assert.NoError(t, err)

		resp := NewInvokeMethodResponse(0, "OK", nil)
		defer resp.Close()
		resp.r.Message.Data = b
		contentType := resp.ContentType()
		bData, err := io.ReadAll(resp.RawData())
		assert.NoError(t, err)
		assert.Equal(t, ProtobufContentType, contentType)
		assert.Equal(t, b.Value, bData)
	})
}

func TestResponseRawData(t *testing.T) {
	t.Run("message is nil", func(t *testing.T) {
		req := &InvokeMethodResponse{
			r: &internalv1pb.InternalInvokeResponse{},
		}
		r := req.RawData()
		assert.Nil(t, r)
	})

	t.Run("return data from stream", func(t *testing.T) {
		req := NewInvokeMethodResponse(0, "OK", nil).
			WithRawDataString("nel blu dipinto di blu")
		defer req.Close()

		r := req.RawData()
		bData, err := io.ReadAll(r)

		assert.NoError(t, err)
		assert.Equal(t, "nel blu dipinto di blu", string(bData))

		_ = assert.Nil(t, req.Message().Data) ||
			assert.Len(t, req.Message().Data.Value, 0)
	})

	t.Run("data inside message has priority", func(t *testing.T) {
		req := NewInvokeMethodResponse(0, "OK", nil).
			WithRawDataString("nel blu dipinto di blu")
		defer req.Close()

		// Override
		const msg = "felice di stare lassu'"
		req.Message().Data = &anypb.Any{Value: []byte(msg)}

		r := req.RawData()
		bData, err := io.ReadAll(r)

		assert.NoError(t, err)
		assert.Equal(t, msg, string(bData))

		_ = assert.NotNil(t, req.Message().Data) &&
			assert.Equal(t, msg, string(req.Message().Data.Value))
	})
}

func TestResponseRawDataFull(t *testing.T) {
	t.Run("message is nil", func(t *testing.T) {
		req := &InvokeMethodResponse{
			r: &internalv1pb.InternalInvokeResponse{},
		}
		data, err := req.RawDataFull()
		assert.NoError(t, err)
		assert.Nil(t, data)
	})

	t.Run("return data from stream", func(t *testing.T) {
		req := NewInvokeMethodResponse(0, "OK", nil).
			WithRawDataString("nel blu dipinto di blu")
		defer req.Close()

		data, err := req.RawDataFull()
		assert.NoError(t, err)
		assert.Equal(t, "nel blu dipinto di blu", string(data))

		_ = assert.Nil(t, req.Message().Data) ||
			assert.Len(t, req.Message().Data.Value, 0)
	})

	t.Run("data inside message has priority", func(t *testing.T) {
		req := NewInvokeMethodResponse(0, "OK", nil).
			WithRawDataString("nel blu dipinto di blu")
		defer req.Close()

		// Override
		const msg = "felice di stare lassu'"
		req.Message().Data = &anypb.Any{Value: []byte(msg)}

		data, err := req.RawDataFull()
		assert.NoError(t, err)
		assert.Equal(t, msg, string(data))

		_ = assert.NotNil(t, req.Message().Data) &&
			assert.Equal(t, msg, string(req.Message().Data.Value))
	})
}

func TestResponseProto(t *testing.T) {
	t.Run("byte slice", func(t *testing.T) {
		m := &commonv1pb.InvokeResponse{
			Data:        &anypb.Any{Value: []byte("test")},
			ContentType: "application/json",
		}
		pb := internalv1pb.InternalInvokeResponse{
			Status:  &internalv1pb.Status{Code: 0},
			Message: m,
		}

		ir, err := InternalInvokeResponse(&pb)
		assert.NoError(t, err)
		defer ir.Close()
		req2 := ir.Proto()
		msg := req2.GetMessage()

		assert.Equal(t, "application/json", msg.ContentType)
		require.NotNil(t, msg.Data)
		require.NotNil(t, msg.Data.Value)
		assert.Equal(t, []byte("test"), msg.Data.Value)

		bData, err := io.ReadAll(ir.RawData())
		assert.NoError(t, err)
		assert.Equal(t, []byte("test"), bData)
	})

	t.Run("stream", func(t *testing.T) {
		m := &commonv1pb.InvokeResponse{
			ContentType: "application/json",
		}
		pb := internalv1pb.InternalInvokeResponse{
			Status:  &internalv1pb.Status{Code: 0},
			Message: m,
		}

		ir, err := InternalInvokeResponse(&pb)
		assert.NoError(t, err)
		defer ir.Close()
		ir.data = io.NopCloser(strings.NewReader("test"))
		req2 := ir.Proto()

		assert.Equal(t, "application/json", req2.Message.ContentType)
		assert.Nil(t, req2.Message.Data)

		bData, err := io.ReadAll(ir.RawData())
		assert.NoError(t, err)
		assert.Equal(t, []byte("test"), bData)
	})
}

func TestResponseProtoWithData(t *testing.T) {
	t.Run("byte slice", func(t *testing.T) {
		m := &commonv1pb.InvokeResponse{
			Data:        &anypb.Any{Value: []byte("test")},
			ContentType: "application/json",
		}
		pb := internalv1pb.InternalInvokeResponse{
			Status:  &internalv1pb.Status{Code: 0},
			Message: m,
		}

		ir, err := InternalInvokeResponse(&pb)
		assert.NoError(t, err)
		defer ir.Close()
		req2, err := ir.ProtoWithData()
		assert.NoError(t, err)

		assert.Equal(t, "application/json", req2.Message.ContentType)
		assert.Equal(t, []byte("test"), req2.Message.Data.Value)
	})

	t.Run("stream", func(t *testing.T) {
		m := &commonv1pb.InvokeResponse{
			ContentType: "application/json",
		}
		pb := internalv1pb.InternalInvokeResponse{
			Status:  &internalv1pb.Status{Code: 0},
			Message: m,
		}

		ir, err := InternalInvokeResponse(&pb)
		assert.NoError(t, err)
		defer ir.Close()
		ir.data = io.NopCloser(strings.NewReader("test"))
		req2, err := ir.ProtoWithData()
		assert.NoError(t, err)

		assert.Equal(t, "application/json", req2.Message.ContentType)
		assert.Equal(t, []byte("test"), req2.Message.Data.Value)
	})
}

func TestResponseHeader(t *testing.T) {
	t.Run("gRPC headers", func(t *testing.T) {
		md := map[string][]string{
			"test1": {"val1", "val2"},
			"test2": {"val3", "val4"},
		}
		imr := NewInvokeMethodResponse(0, "OK", nil).
			WithHeaders(md)
		defer imr.Close()
		mheader := imr.Headers()

		assert.Equal(t, "val1", mheader["test1"].Values[0])
		assert.Equal(t, "val2", mheader["test1"].Values[1])
		assert.Equal(t, "val3", mheader["test2"].Values[0])
		assert.Equal(t, "val4", mheader["test2"].Values[1])
	})

	t.Run("HTTP headers", func(t *testing.T) {
		resp := fasthttp.AcquireResponse()
		resp.Header.Set("Header1", "Value1")
		resp.Header.Set("Header2", "Value2")
		resp.Header.Set("Header3", "Value3")

		imr := NewInvokeMethodResponse(0, "OK", nil).
			WithFastHTTPHeaders(&resp.Header)
		defer imr.Close()
		mheader := imr.Headers()

		assert.Equal(t, "Value1", mheader["Header1"].GetValues()[0])
		assert.Equal(t, "Value2", mheader["Header2"].GetValues()[0])
		assert.Equal(t, "Value3", mheader["Header3"].GetValues()[0])
	})
}

func TestResponseTrailer(t *testing.T) {
	md := map[string][]string{
		"test1": {"val1", "val2"},
		"test2": {"val3", "val4"},
	}
	resp := NewInvokeMethodResponse(0, "OK", nil).
		WithTrailers(md)
	defer resp.Close()
	mheader := resp.Trailers()

	assert.Equal(t, "val1", mheader["test1"].GetValues()[0])
	assert.Equal(t, "val2", mheader["test1"].GetValues()[1])
	assert.Equal(t, "val3", mheader["test2"].GetValues()[0])
	assert.Equal(t, "val4", mheader["test2"].GetValues()[1])
}

func TestIsHTTPResponse(t *testing.T) {
	t.Run("gRPC response status", func(t *testing.T) {
		imr := NewInvokeMethodResponse(int32(codes.OK), "OK", nil)
		defer imr.Close()
		assert.False(t, imr.IsHTTPResponse())
	})

	t.Run("HTTP response status", func(t *testing.T) {
		imr := NewInvokeMethodResponse(http.StatusOK, "OK", nil)
		defer imr.Close()
		assert.True(t, imr.IsHTTPResponse())
	})
}

func TestResponseReplayable(t *testing.T) {
	const message = "Nel mezzo del cammin di nostra vita mi ritrovai per una selva oscura, che' la diritta via era smarrita."
	newReplayable := func() *InvokeMethodResponse {
		m := &commonv1pb.InvokeResponse{
			Data:        &anypb.Any{Value: []byte("test")},
			ContentType: "application/json",
		}
		pb := internalv1pb.InternalInvokeResponse{
			Status:  &internalv1pb.Status{Code: 0},
			Message: m,
		}

		res, _ := InternalInvokeResponse(&pb)
		return res.
			WithRawDataString(message).
			WithReplay(true)
	}

	t.Run("read once", func(t *testing.T) {
		res := newReplayable()
		defer res.Close()

		t.Run("first read in full", func(t *testing.T) {
			read, err := io.ReadAll(res.RawData())
			assert.NoError(t, err)
			assert.Equal(t, message, string(read))
		})

		t.Run("req.data is EOF", func(t *testing.T) {
			buf := make([]byte, 9)
			n, err := io.ReadFull(res.data, buf)
			assert.Equal(t, 0, n)
			assert.ErrorIs(t, err, io.EOF)
		})

		t.Run("replay buffer is full", func(t *testing.T) {
			assert.Equal(t, len(message), res.replay.Len())
			read, err := io.ReadAll(bytes.NewReader(res.replay.Bytes()))
			assert.NoError(t, err)
			assert.Equal(t, message, string(read))
		})

		t.Run("close response", func(t *testing.T) {
			err := res.Close()
			assert.NoError(t, err)
			assert.Nil(t, res.data)
			assert.Nil(t, res.replay)
		})
	})

	t.Run("read in full three times", func(t *testing.T) {
		res := newReplayable()
		defer res.Close()

		t.Run("first read in full", func(t *testing.T) {
			read, err := io.ReadAll(res.RawData())
			assert.NoError(t, err)
			assert.Equal(t, message, string(read))
		})

		t.Run("req.data is EOF", func(t *testing.T) {
			buf := make([]byte, 9)
			n, err := io.ReadFull(res.data, buf)
			assert.Equal(t, 0, n)
			assert.ErrorIs(t, err, io.EOF)
		})

		t.Run("replay buffer is full", func(t *testing.T) {
			assert.Equal(t, len(message), res.replay.Len())
			read, err := io.ReadAll(bytes.NewReader(res.replay.Bytes()))
			assert.NoError(t, err)
			assert.Equal(t, message, string(read))
		})

		t.Run("second read in full", func(t *testing.T) {
			read, err := io.ReadAll(res.RawData())
			assert.NoError(t, err)
			assert.Equal(t, message, string(read))
		})

		t.Run("third read in full", func(t *testing.T) {
			read, err := io.ReadAll(res.RawData())
			assert.NoError(t, err)
			assert.Equal(t, message, string(read))
		})

		t.Run("close response", func(t *testing.T) {
			err := res.Close()
			assert.NoError(t, err)
			assert.Nil(t, res.data)
			assert.Nil(t, res.replay)
		})
	})

	t.Run("read in full, then partial read", func(t *testing.T) {
		res := newReplayable()
		defer res.Close()

		t.Run("first read in full", func(t *testing.T) {
			read, err := io.ReadAll(res.RawData())
			assert.NoError(t, err)
			assert.Equal(t, message, string(read))
		})

		r := res.RawData()
		t.Run("second, partial read", func(t *testing.T) {
			buf := make([]byte, 9)
			n, err := io.ReadFull(r, buf)
			assert.NoError(t, err)
			assert.Equal(t, 9, n)
			assert.Equal(t, message[:9], string(buf))
		})

		t.Run("read rest", func(t *testing.T) {
			read, err := io.ReadAll(r)
			assert.NoError(t, err)
			assert.Len(t, read, len(message)-9)
			// Continue from byte 9
			assert.Equal(t, message[9:], string(read))
		})

		t.Run("second read in full", func(t *testing.T) {
			read, err := res.RawDataFull()
			assert.NoError(t, err)
			assert.Equal(t, message, string(read))
		})

		t.Run("close response", func(t *testing.T) {
			err := res.Close()
			assert.NoError(t, err)
			assert.Nil(t, res.data)
			assert.Nil(t, res.replay)
		})
	})

	t.Run("partial read, then read in full", func(t *testing.T) {
		res := newReplayable()
		defer res.Close()

		t.Run("first, partial read", func(t *testing.T) {
			buf := make([]byte, 9)
			n, err := io.ReadFull(res.RawData(), buf)

			assert.NoError(t, err)
			assert.Equal(t, 9, n)
			assert.Equal(t, message[:9], string(buf))
		})

		t.Run("replay buffer has partial data", func(t *testing.T) {
			assert.Equal(t, 9, res.replay.Len())
			read, err := io.ReadAll(bytes.NewReader(res.replay.Bytes()))
			assert.NoError(t, err)
			assert.Equal(t, message[:9], string(read))
		})

		t.Run("second read in full", func(t *testing.T) {
			read, err := io.ReadAll(res.RawData())
			assert.NoError(t, err)
			assert.Equal(t, message, string(read))
		})

		t.Run("req.data is EOF", func(t *testing.T) {
			buf := make([]byte, 9)
			n, err := io.ReadFull(res.data, buf)
			assert.Equal(t, 0, n)
			assert.ErrorIs(t, err, io.EOF)
		})

		t.Run("replay buffer is full", func(t *testing.T) {
			assert.Equal(t, len(message), res.replay.Len())
			read, err := io.ReadAll(bytes.NewReader(res.replay.Bytes()))
			assert.NoError(t, err)
			assert.Equal(t, message, string(read))
		})

		t.Run("third read in full", func(t *testing.T) {
			read, err := io.ReadAll(res.RawData())
			assert.NoError(t, err)
			assert.Equal(t, message, string(read))
		})

		t.Run("close response", func(t *testing.T) {
			err := res.Close()
			assert.NoError(t, err)
			assert.Nil(t, res.data)
			assert.Nil(t, res.replay)
		})
	})

	t.Run("get ProtoWithData twice", func(t *testing.T) {
		res := newReplayable()
		defer res.Close()

		t.Run("first ProtoWithData response", func(t *testing.T) {
			pb, err := res.ProtoWithData()
			assert.NoError(t, err)
			assert.NotNil(t, pb)
			assert.NotNil(t, pb.Message)
			assert.NotNil(t, pb.Message.Data)
			assert.Equal(t, message, string(pb.Message.Data.Value))
		})

		t.Run("second ProtoWithData response", func(t *testing.T) {
			pb, err := res.ProtoWithData()
			assert.NoError(t, err)
			assert.NotNil(t, pb)
			assert.NotNil(t, pb.Message)
			assert.NotNil(t, pb.Message.Data)
			assert.Equal(t, message, string(pb.Message.Data.Value))
		})

		t.Run("close response", func(t *testing.T) {
			err := res.Close()
			assert.NoError(t, err)
			assert.Nil(t, res.data)
			assert.Nil(t, res.replay)
		})
	})
}
