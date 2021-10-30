// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: proto/blockchain/msgs.proto

package blockchain

import (
	fmt "fmt"
	_ "github.com/gogo/protobuf/gogoproto"
	proto "github.com/gogo/protobuf/proto"
	types "github.com/okex/exchain/dependence/tendermint/proto/types"
	math "math"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.GoGoProtoPackageIsVersion3 // please upgrade the proto package

// BlockRequest requests a block for a specific height
type BlockRequest struct {
	Height               int64    `protobuf:"varint,1,opt,name=height,proto3" json:"height,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *BlockRequest) Reset()         { *m = BlockRequest{} }
func (m *BlockRequest) String() string { return proto.CompactTextString(m) }
func (*BlockRequest) ProtoMessage()    {}
func (*BlockRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_ecf660069f8bb334, []int{0}
}
func (m *BlockRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_BlockRequest.Unmarshal(m, b)
}
func (m *BlockRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_BlockRequest.Marshal(b, m, deterministic)
}
func (m *BlockRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_BlockRequest.Merge(m, src)
}
func (m *BlockRequest) XXX_Size() int {
	return xxx_messageInfo_BlockRequest.Size(m)
}
func (m *BlockRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_BlockRequest.DiscardUnknown(m)
}

var xxx_messageInfo_BlockRequest proto.InternalMessageInfo

func (m *BlockRequest) GetHeight() int64 {
	if m != nil {
		return m.Height
	}
	return 0
}

// NoBlockResponse informs the node that the peer does not have block at the requested height
type NoBlockResponse struct {
	Height               int64    `protobuf:"varint,1,opt,name=height,proto3" json:"height,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *NoBlockResponse) Reset()         { *m = NoBlockResponse{} }
func (m *NoBlockResponse) String() string { return proto.CompactTextString(m) }
func (*NoBlockResponse) ProtoMessage()    {}
func (*NoBlockResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_ecf660069f8bb334, []int{1}
}
func (m *NoBlockResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_NoBlockResponse.Unmarshal(m, b)
}
func (m *NoBlockResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_NoBlockResponse.Marshal(b, m, deterministic)
}
func (m *NoBlockResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_NoBlockResponse.Merge(m, src)
}
func (m *NoBlockResponse) XXX_Size() int {
	return xxx_messageInfo_NoBlockResponse.Size(m)
}
func (m *NoBlockResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_NoBlockResponse.DiscardUnknown(m)
}

var xxx_messageInfo_NoBlockResponse proto.InternalMessageInfo

func (m *NoBlockResponse) GetHeight() int64 {
	if m != nil {
		return m.Height
	}
	return 0
}

// BlockResponse returns block to the requested
type BlockResponse struct {
	Block                types.Block `protobuf:"bytes,1,opt,name=block,proto3" json:"block"`
	XXX_NoUnkeyedLiteral struct{}    `json:"-"`
	XXX_unrecognized     []byte      `json:"-"`
	XXX_sizecache        int32       `json:"-"`
}

func (m *BlockResponse) Reset()         { *m = BlockResponse{} }
func (m *BlockResponse) String() string { return proto.CompactTextString(m) }
func (*BlockResponse) ProtoMessage()    {}
func (*BlockResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_ecf660069f8bb334, []int{2}
}
func (m *BlockResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_BlockResponse.Unmarshal(m, b)
}
func (m *BlockResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_BlockResponse.Marshal(b, m, deterministic)
}
func (m *BlockResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_BlockResponse.Merge(m, src)
}
func (m *BlockResponse) XXX_Size() int {
	return xxx_messageInfo_BlockResponse.Size(m)
}
func (m *BlockResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_BlockResponse.DiscardUnknown(m)
}

var xxx_messageInfo_BlockResponse proto.InternalMessageInfo

func (m *BlockResponse) GetBlock() types.Block {
	if m != nil {
		return m.Block
	}
	return types.Block{}
}

// StatusRequest requests the status of a node (Height & Base)
type StatusRequest struct {
	Height               int64    `protobuf:"varint,1,opt,name=height,proto3" json:"height,omitempty"`
	Base                 int64    `protobuf:"varint,2,opt,name=base,proto3" json:"base,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *StatusRequest) Reset()         { *m = StatusRequest{} }
func (m *StatusRequest) String() string { return proto.CompactTextString(m) }
func (*StatusRequest) ProtoMessage()    {}
func (*StatusRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_ecf660069f8bb334, []int{3}
}
func (m *StatusRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_StatusRequest.Unmarshal(m, b)
}
func (m *StatusRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_StatusRequest.Marshal(b, m, deterministic)
}
func (m *StatusRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_StatusRequest.Merge(m, src)
}
func (m *StatusRequest) XXX_Size() int {
	return xxx_messageInfo_StatusRequest.Size(m)
}
func (m *StatusRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_StatusRequest.DiscardUnknown(m)
}

var xxx_messageInfo_StatusRequest proto.InternalMessageInfo

func (m *StatusRequest) GetHeight() int64 {
	if m != nil {
		return m.Height
	}
	return 0
}

func (m *StatusRequest) GetBase() int64 {
	if m != nil {
		return m.Base
	}
	return 0
}

// StatusResponse is a peer response to infrom their status
type StatusResponse struct {
	Height               int64    `protobuf:"varint,1,opt,name=height,proto3" json:"height,omitempty"`
	Base                 int64    `protobuf:"varint,2,opt,name=base,proto3" json:"base,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *StatusResponse) Reset()         { *m = StatusResponse{} }
func (m *StatusResponse) String() string { return proto.CompactTextString(m) }
func (*StatusResponse) ProtoMessage()    {}
func (*StatusResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_ecf660069f8bb334, []int{4}
}
func (m *StatusResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_StatusResponse.Unmarshal(m, b)
}
func (m *StatusResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_StatusResponse.Marshal(b, m, deterministic)
}
func (m *StatusResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_StatusResponse.Merge(m, src)
}
func (m *StatusResponse) XXX_Size() int {
	return xxx_messageInfo_StatusResponse.Size(m)
}
func (m *StatusResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_StatusResponse.DiscardUnknown(m)
}

var xxx_messageInfo_StatusResponse proto.InternalMessageInfo

func (m *StatusResponse) GetHeight() int64 {
	if m != nil {
		return m.Height
	}
	return 0
}

func (m *StatusResponse) GetBase() int64 {
	if m != nil {
		return m.Base
	}
	return 0
}

type Message struct {
	// Types that are valid to be assigned to Sum:
	//	*Message_BlockRequest
	//	*Message_NoBlockResponse
	//	*Message_BlockResponse
	//	*Message_StatusRequest
	//	*Message_StatusResponse
	Sum                  isMessage_Sum `protobuf_oneof:"sum"`
	XXX_NoUnkeyedLiteral struct{}      `json:"-"`
	XXX_unrecognized     []byte        `json:"-"`
	XXX_sizecache        int32         `json:"-"`
}

func (m *Message) Reset()         { *m = Message{} }
func (m *Message) String() string { return proto.CompactTextString(m) }
func (*Message) ProtoMessage()    {}
func (*Message) Descriptor() ([]byte, []int) {
	return fileDescriptor_ecf660069f8bb334, []int{5}
}
func (m *Message) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Message.Unmarshal(m, b)
}
func (m *Message) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Message.Marshal(b, m, deterministic)
}
func (m *Message) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Message.Merge(m, src)
}
func (m *Message) XXX_Size() int {
	return xxx_messageInfo_Message.Size(m)
}
func (m *Message) XXX_DiscardUnknown() {
	xxx_messageInfo_Message.DiscardUnknown(m)
}

var xxx_messageInfo_Message proto.InternalMessageInfo

type isMessage_Sum interface {
	isMessage_Sum()
}

type Message_BlockRequest struct {
	BlockRequest *BlockRequest `protobuf:"bytes,1,opt,name=block_request,json=blockRequest,proto3,oneof" json:"block_request,omitempty"`
}
type Message_NoBlockResponse struct {
	NoBlockResponse *NoBlockResponse `protobuf:"bytes,2,opt,name=no_block_response,json=noBlockResponse,proto3,oneof" json:"no_block_response,omitempty"`
}
type Message_BlockResponse struct {
	BlockResponse *BlockResponse `protobuf:"bytes,3,opt,name=block_response,json=blockResponse,proto3,oneof" json:"block_response,omitempty"`
}
type Message_StatusRequest struct {
	StatusRequest *StatusRequest `protobuf:"bytes,4,opt,name=status_request,json=statusRequest,proto3,oneof" json:"status_request,omitempty"`
}
type Message_StatusResponse struct {
	StatusResponse *StatusResponse `protobuf:"bytes,5,opt,name=status_response,json=statusResponse,proto3,oneof" json:"status_response,omitempty"`
}

func (*Message_BlockRequest) isMessage_Sum()    {}
func (*Message_NoBlockResponse) isMessage_Sum() {}
func (*Message_BlockResponse) isMessage_Sum()   {}
func (*Message_StatusRequest) isMessage_Sum()   {}
func (*Message_StatusResponse) isMessage_Sum()  {}

func (m *Message) GetSum() isMessage_Sum {
	if m != nil {
		return m.Sum
	}
	return nil
}

func (m *Message) GetBlockRequest() *BlockRequest {
	if x, ok := m.GetSum().(*Message_BlockRequest); ok {
		return x.BlockRequest
	}
	return nil
}

func (m *Message) GetNoBlockResponse() *NoBlockResponse {
	if x, ok := m.GetSum().(*Message_NoBlockResponse); ok {
		return x.NoBlockResponse
	}
	return nil
}

func (m *Message) GetBlockResponse() *BlockResponse {
	if x, ok := m.GetSum().(*Message_BlockResponse); ok {
		return x.BlockResponse
	}
	return nil
}

func (m *Message) GetStatusRequest() *StatusRequest {
	if x, ok := m.GetSum().(*Message_StatusRequest); ok {
		return x.StatusRequest
	}
	return nil
}

func (m *Message) GetStatusResponse() *StatusResponse {
	if x, ok := m.GetSum().(*Message_StatusResponse); ok {
		return x.StatusResponse
	}
	return nil
}

// XXX_OneofWrappers is for the internal use of the proto package.
func (*Message) XXX_OneofWrappers() []interface{} {
	return []interface{}{
		(*Message_BlockRequest)(nil),
		(*Message_NoBlockResponse)(nil),
		(*Message_BlockResponse)(nil),
		(*Message_StatusRequest)(nil),
		(*Message_StatusResponse)(nil),
	}
}

func init() {
	proto.RegisterType((*BlockRequest)(nil), "tendermint.proto.blockchain.BlockRequest")
	proto.RegisterType((*NoBlockResponse)(nil), "tendermint.proto.blockchain.NoBlockResponse")
	proto.RegisterType((*BlockResponse)(nil), "tendermint.proto.blockchain.BlockResponse")
	proto.RegisterType((*StatusRequest)(nil), "tendermint.proto.blockchain.StatusRequest")
	proto.RegisterType((*StatusResponse)(nil), "tendermint.proto.blockchain.StatusResponse")
	proto.RegisterType((*Message)(nil), "tendermint.proto.blockchain.Message")
}

func init() { proto.RegisterFile("proto/blockchain/msgs.proto", fileDescriptor_ecf660069f8bb334) }

var fileDescriptor_ecf660069f8bb334 = []byte{
	// 369 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x8c, 0x93, 0xc1, 0x4e, 0xc2, 0x40,
	0x10, 0x86, 0xc1, 0x02, 0x26, 0x03, 0x85, 0xd8, 0x83, 0x12, 0x88, 0xd1, 0xf4, 0x40, 0x44, 0xcd,
	0x36, 0xc2, 0xc9, 0xe8, 0xa9, 0x27, 0x62, 0xa2, 0x31, 0x25, 0xf1, 0xc0, 0x85, 0xb4, 0xb0, 0x69,
	0x1b, 0x6d, 0xb7, 0x76, 0xb6, 0x07, 0xde, 0xce, 0xa3, 0x4f, 0xe1, 0xb3, 0x18, 0x76, 0x4b, 0xa1,
	0x55, 0xb1, 0xb7, 0xdd, 0xbf, 0x33, 0xdf, 0xfc, 0x3b, 0x7f, 0x0a, 0xfd, 0x28, 0x66, 0x9c, 0x19,
	0xce, 0x1b, 0x5b, 0xbc, 0x2e, 0x3c, 0xdb, 0x0f, 0x8d, 0x00, 0x5d, 0x24, 0x42, 0xd5, 0xfa, 0x9c,
	0x86, 0x4b, 0x1a, 0x07, 0x7e, 0xc8, 0xa5, 0x42, 0xb6, 0x75, 0xbd, 0x01, 0xf7, 0xfc, 0x78, 0x39,
	0x8f, 0xec, 0x98, 0xaf, 0x0c, 0x49, 0x71, 0x99, 0xcb, 0xb6, 0x27, 0xd9, 0xd2, 0x3b, 0x91, 0x0a,
	0x5f, 0x45, 0x14, 0xe5, 0x1c, 0xf9, 0x41, 0x1f, 0x40, 0xcb, 0x5c, 0x5f, 0x2d, 0xfa, 0x9e, 0x50,
	0xe4, 0xda, 0x31, 0x34, 0x3c, 0xea, 0xbb, 0x1e, 0xef, 0x56, 0xcf, 0xab, 0x17, 0x8a, 0x95, 0xde,
	0xf4, 0x21, 0x74, 0x9e, 0x58, 0x5a, 0x89, 0x11, 0x0b, 0x91, 0xfe, 0x59, 0xfa, 0x00, 0x6a, 0xbe,
	0xf0, 0x16, 0xea, 0x62, 0xa4, 0xa8, 0x6b, 0x8e, 0x4e, 0xc9, 0x8f, 0x17, 0x09, 0x5f, 0x44, 0x74,
	0x99, 0xb5, 0xcf, 0xaf, 0xb3, 0x8a, 0x25, 0x3b, 0xf4, 0x3b, 0x50, 0xa7, 0xdc, 0xe6, 0x09, 0xfe,
	0xe3, 0x4f, 0xd3, 0xa0, 0xe6, 0xd8, 0x48, 0xbb, 0x07, 0x42, 0x15, 0x67, 0xfd, 0x1e, 0xda, 0x9b,
	0xe6, 0xfd, 0x96, 0x7f, 0xed, 0xfe, 0x50, 0xe0, 0xf0, 0x91, 0x22, 0xda, 0x2e, 0xd5, 0x9e, 0x41,
	0x15, 0x7e, 0xe6, 0xb1, 0xb4, 0x91, 0xbe, 0x64, 0x48, 0xf6, 0x64, 0x43, 0x76, 0xf7, 0x3a, 0xa9,
	0x58, 0x2d, 0x67, 0x77, 0xcf, 0x33, 0x38, 0x0a, 0xd9, 0x7c, 0x03, 0x95, 0xf6, 0xc4, 0xf8, 0xe6,
	0xe8, 0x7a, 0x2f, 0xb5, 0x90, 0xc2, 0xa4, 0x62, 0x75, 0xc2, 0x42, 0x30, 0x53, 0x68, 0x17, 0xc0,
	0x8a, 0x00, 0x5f, 0x96, 0xb1, 0x9b, 0x61, 0x55, 0xa7, 0x08, 0x45, 0xb1, 0xcc, 0x6c, 0x07, 0xb5,
	0x12, 0xd0, 0x5c, 0x78, 0x6b, 0x28, 0xe6, 0xd2, 0x7c, 0x81, 0x4e, 0x06, 0x4d, 0xad, 0xd6, 0x05,
	0xf5, 0xaa, 0x14, 0x35, 0xf3, 0xda, 0xc6, 0x9c, 0x62, 0xd6, 0x41, 0xc1, 0x24, 0x30, 0xc7, 0xb3,
	0x1b, 0xd7, 0xe7, 0x5e, 0xe2, 0x90, 0x05, 0x0b, 0x8c, 0x2d, 0x71, 0xf7, 0x58, 0xfc, 0xf5, 0x9c,
	0x86, 0x50, 0xc6, 0xdf, 0x01, 0x00, 0x00, 0xff, 0xff, 0xb8, 0xb9, 0x72, 0x28, 0x95, 0x03, 0x00,
	0x00,
}
