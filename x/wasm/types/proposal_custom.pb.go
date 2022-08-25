// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: x/wasm/proto/proposal_custom.proto

package types

import (
	fmt "fmt"
	proto "github.com/gogo/protobuf/proto"
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

type UpdateDeploymentWhitelistProposal struct {
	Title                string   `protobuf:"bytes,1,opt,name=title,proto3" json:"title,omitempty"`
	Description          string   `protobuf:"bytes,2,opt,name=description,proto3" json:"description,omitempty"`
	DistributorAddrs     []string `protobuf:"bytes,3,rep,name=distributorAddrs,proto3" json:"distributorAddrs,omitempty"`
	IsAdded              bool     `protobuf:"varint,4,opt,name=isAdded,proto3" json:"isAdded,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *UpdateDeploymentWhitelistProposal) Reset()         { *m = UpdateDeploymentWhitelistProposal{} }
func (m *UpdateDeploymentWhitelistProposal) String() string { return proto.CompactTextString(m) }
func (*UpdateDeploymentWhitelistProposal) ProtoMessage()    {}
func (*UpdateDeploymentWhitelistProposal) Descriptor() ([]byte, []int) {
	return fileDescriptor_dd9d4d6e8a1d82c0, []int{0}
}
func (m *UpdateDeploymentWhitelistProposal) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_UpdateDeploymentWhitelistProposal.Unmarshal(m, b)
}
func (m *UpdateDeploymentWhitelistProposal) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_UpdateDeploymentWhitelistProposal.Marshal(b, m, deterministic)
}
func (m *UpdateDeploymentWhitelistProposal) XXX_Merge(src proto.Message) {
	xxx_messageInfo_UpdateDeploymentWhitelistProposal.Merge(m, src)
}
func (m *UpdateDeploymentWhitelistProposal) XXX_Size() int {
	return xxx_messageInfo_UpdateDeploymentWhitelistProposal.Size(m)
}
func (m *UpdateDeploymentWhitelistProposal) XXX_DiscardUnknown() {
	xxx_messageInfo_UpdateDeploymentWhitelistProposal.DiscardUnknown(m)
}

var xxx_messageInfo_UpdateDeploymentWhitelistProposal proto.InternalMessageInfo

func (m *UpdateDeploymentWhitelistProposal) GetTitle() string {
	if m != nil {
		return m.Title
	}
	return ""
}

func (m *UpdateDeploymentWhitelistProposal) GetDescription() string {
	if m != nil {
		return m.Description
	}
	return ""
}

func (m *UpdateDeploymentWhitelistProposal) GetDistributorAddrs() []string {
	if m != nil {
		return m.DistributorAddrs
	}
	return nil
}

func (m *UpdateDeploymentWhitelistProposal) GetIsAdded() bool {
	if m != nil {
		return m.IsAdded
	}
	return false
}

func init() {
	proto.RegisterType((*UpdateDeploymentWhitelistProposal)(nil), "types.UpdateDeploymentWhitelistProposal")
}

func init() {
	proto.RegisterFile("x/wasm/proto/proposal_custom.proto", fileDescriptor_dd9d4d6e8a1d82c0)
}

var fileDescriptor_dd9d4d6e8a1d82c0 = []byte{
	// 195 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x64, 0x8f, 0xc1, 0x4a, 0xc6, 0x30,
	0x0c, 0x80, 0xa9, 0x73, 0xea, 0xaa, 0x88, 0x14, 0x0f, 0x3d, 0xd6, 0x9d, 0x86, 0x07, 0x77, 0xf0,
	0x09, 0x26, 0x3e, 0x80, 0x0c, 0x44, 0xf0, 0x22, 0xdb, 0x12, 0x30, 0xd0, 0xad, 0xa5, 0xc9, 0xd0,
	0x3d, 0x8e, 0x6f, 0x2a, 0xd6, 0x09, 0xc2, 0x7f, 0x09, 0x7c, 0x5f, 0x08, 0xe4, 0xd3, 0xf5, 0x67,
	0xfb, 0x31, 0xf0, 0xdc, 0xc6, 0x14, 0x24, 0xfc, 0xcc, 0x18, 0x78, 0xf0, 0x6f, 0xd3, 0xca, 0x12,
	0xe6, 0xbb, 0x6c, 0x4d, 0x29, 0x5b, 0x44, 0xae, 0xbf, 0x94, 0xbe, 0x79, 0x8e, 0x30, 0x08, 0x3e,
	0x62, 0xf4, 0x61, 0x9b, 0x71, 0x91, 0x97, 0x77, 0x12, 0xf4, 0xc4, 0xf2, 0xb4, 0x5f, 0x9a, 0x6b,
	0x5d, 0x0a, 0x89, 0x47, 0xab, 0x9c, 0x6a, 0xaa, 0xfe, 0x17, 0x8c, 0xd3, 0xe7, 0x80, 0x3c, 0x25,
	0x8a, 0x42, 0x61, 0xb1, 0x47, 0x79, 0xf7, 0x5f, 0x99, 0x5b, 0x7d, 0x05, 0xc4, 0x92, 0x68, 0x5c,
	0x25, 0xa4, 0x0e, 0x20, 0xb1, 0x2d, 0x5c, 0xd1, 0x54, 0xfd, 0x81, 0x37, 0x56, 0x9f, 0x12, 0x77,
	0x00, 0x08, 0xf6, 0xd8, 0xa9, 0xe6, 0xac, 0xff, 0xc3, 0x87, 0xcb, 0xd7, 0x8b, 0x3d, 0x28, 0xff,
	0x3c, 0x9e, 0xe4, 0x82, 0xfb, 0xef, 0x00, 0x00, 0x00, 0xff, 0xff, 0x99, 0xea, 0x26, 0x69, 0xe7,
	0x00, 0x00, 0x00,
}
