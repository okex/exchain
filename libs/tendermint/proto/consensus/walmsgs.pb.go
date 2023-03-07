// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: proto/consensus/walmsgs.proto

package consensus

import (
	fmt "fmt"
	_ "github.com/gogo/protobuf/gogoproto"
	proto "github.com/gogo/protobuf/proto"
	_ "github.com/golang/protobuf/ptypes/duration"
	_ "github.com/golang/protobuf/ptypes/timestamp"
	types "github.com/okx/exchain/libs/tendermint/proto/types"
	math "math"
	time "time"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf
var _ = time.Kitchen

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.GoGoProtoPackageIsVersion3 // please upgrade the proto package

// MsgInfo are msgs from the reactor which may update the state
type MsgInfo struct {
	Msg                  Message  `protobuf:"bytes,1,opt,name=msg,proto3" json:"msg"`
	PeerID               string   `protobuf:"bytes,2,opt,name=peer_id,json=peerId,proto3" json:"peer_id,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *MsgInfo) Reset()         { *m = MsgInfo{} }
func (m *MsgInfo) String() string { return proto.CompactTextString(m) }
func (*MsgInfo) ProtoMessage()    {}
func (*MsgInfo) Descriptor() ([]byte, []int) {
	return fileDescriptor_60ad80fa14e37285, []int{0}
}
func (m *MsgInfo) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_MsgInfo.Unmarshal(m, b)
}
func (m *MsgInfo) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_MsgInfo.Marshal(b, m, deterministic)
}
func (m *MsgInfo) XXX_Merge(src proto.Message) {
	xxx_messageInfo_MsgInfo.Merge(m, src)
}
func (m *MsgInfo) XXX_Size() int {
	return xxx_messageInfo_MsgInfo.Size(m)
}
func (m *MsgInfo) XXX_DiscardUnknown() {
	xxx_messageInfo_MsgInfo.DiscardUnknown(m)
}

var xxx_messageInfo_MsgInfo proto.InternalMessageInfo

func (m *MsgInfo) GetMsg() Message {
	if m != nil {
		return m.Msg
	}
	return Message{}
}

func (m *MsgInfo) GetPeerID() string {
	if m != nil {
		return m.PeerID
	}
	return ""
}

// TimeoutInfo internally generated messages which may update the state
type TimeoutInfo struct {
	Duration             time.Duration `protobuf:"bytes,1,opt,name=duration,proto3,stdduration" json:"duration"`
	Height               int64         `protobuf:"varint,2,opt,name=height,proto3" json:"height,omitempty"`
	Round                int32         `protobuf:"varint,3,opt,name=round,proto3" json:"round,omitempty"`
	Step                 uint32        `protobuf:"varint,4,opt,name=step,proto3" json:"step,omitempty"`
	XXX_NoUnkeyedLiteral struct{}      `json:"-"`
	XXX_unrecognized     []byte        `json:"-"`
	XXX_sizecache        int32         `json:"-"`
}

func (m *TimeoutInfo) Reset()         { *m = TimeoutInfo{} }
func (m *TimeoutInfo) String() string { return proto.CompactTextString(m) }
func (*TimeoutInfo) ProtoMessage()    {}
func (*TimeoutInfo) Descriptor() ([]byte, []int) {
	return fileDescriptor_60ad80fa14e37285, []int{1}
}
func (m *TimeoutInfo) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_TimeoutInfo.Unmarshal(m, b)
}
func (m *TimeoutInfo) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_TimeoutInfo.Marshal(b, m, deterministic)
}
func (m *TimeoutInfo) XXX_Merge(src proto.Message) {
	xxx_messageInfo_TimeoutInfo.Merge(m, src)
}
func (m *TimeoutInfo) XXX_Size() int {
	return xxx_messageInfo_TimeoutInfo.Size(m)
}
func (m *TimeoutInfo) XXX_DiscardUnknown() {
	xxx_messageInfo_TimeoutInfo.DiscardUnknown(m)
}

var xxx_messageInfo_TimeoutInfo proto.InternalMessageInfo

func (m *TimeoutInfo) GetDuration() time.Duration {
	if m != nil {
		return m.Duration
	}
	return 0
}

func (m *TimeoutInfo) GetHeight() int64 {
	if m != nil {
		return m.Height
	}
	return 0
}

func (m *TimeoutInfo) GetRound() int32 {
	if m != nil {
		return m.Round
	}
	return 0
}

func (m *TimeoutInfo) GetStep() uint32 {
	if m != nil {
		return m.Step
	}
	return 0
}

// EndHeightMessage marks the end of the given height inside WAL.
// @internal used by scripts/wal2json util.
type EndHeight struct {
	Height               int64    `protobuf:"varint,1,opt,name=height,proto3" json:"height,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *EndHeight) Reset()         { *m = EndHeight{} }
func (m *EndHeight) String() string { return proto.CompactTextString(m) }
func (*EndHeight) ProtoMessage()    {}
func (*EndHeight) Descriptor() ([]byte, []int) {
	return fileDescriptor_60ad80fa14e37285, []int{2}
}
func (m *EndHeight) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_EndHeight.Unmarshal(m, b)
}
func (m *EndHeight) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_EndHeight.Marshal(b, m, deterministic)
}
func (m *EndHeight) XXX_Merge(src proto.Message) {
	xxx_messageInfo_EndHeight.Merge(m, src)
}
func (m *EndHeight) XXX_Size() int {
	return xxx_messageInfo_EndHeight.Size(m)
}
func (m *EndHeight) XXX_DiscardUnknown() {
	xxx_messageInfo_EndHeight.DiscardUnknown(m)
}

var xxx_messageInfo_EndHeight proto.InternalMessageInfo

func (m *EndHeight) GetHeight() int64 {
	if m != nil {
		return m.Height
	}
	return 0
}

type WALMessage struct {
	// Types that are valid to be assigned to Sum:
	//	*WALMessage_EventDataRoundState
	//	*WALMessage_MsgInfo
	//	*WALMessage_TimeoutInfo
	//	*WALMessage_EndHeight
	Sum                  isWALMessage_Sum `protobuf_oneof:"sum"`
	XXX_NoUnkeyedLiteral struct{}         `json:"-"`
	XXX_unrecognized     []byte           `json:"-"`
	XXX_sizecache        int32            `json:"-"`
}

func (m *WALMessage) Reset()         { *m = WALMessage{} }
func (m *WALMessage) String() string { return proto.CompactTextString(m) }
func (*WALMessage) ProtoMessage()    {}
func (*WALMessage) Descriptor() ([]byte, []int) {
	return fileDescriptor_60ad80fa14e37285, []int{3}
}
func (m *WALMessage) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_WALMessage.Unmarshal(m, b)
}
func (m *WALMessage) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_WALMessage.Marshal(b, m, deterministic)
}
func (m *WALMessage) XXX_Merge(src proto.Message) {
	xxx_messageInfo_WALMessage.Merge(m, src)
}
func (m *WALMessage) XXX_Size() int {
	return xxx_messageInfo_WALMessage.Size(m)
}
func (m *WALMessage) XXX_DiscardUnknown() {
	xxx_messageInfo_WALMessage.DiscardUnknown(m)
}

var xxx_messageInfo_WALMessage proto.InternalMessageInfo

type isWALMessage_Sum interface {
	isWALMessage_Sum()
}

type WALMessage_EventDataRoundState struct {
	EventDataRoundState *types.EventDataRoundState `protobuf:"bytes,1,opt,name=event_data_round_state,json=eventDataRoundState,proto3,oneof" json:"event_data_round_state,omitempty"`
}
type WALMessage_MsgInfo struct {
	MsgInfo *MsgInfo `protobuf:"bytes,2,opt,name=msg_info,json=msgInfo,proto3,oneof" json:"msg_info,omitempty"`
}
type WALMessage_TimeoutInfo struct {
	TimeoutInfo *TimeoutInfo `protobuf:"bytes,3,opt,name=timeout_info,json=timeoutInfo,proto3,oneof" json:"timeout_info,omitempty"`
}
type WALMessage_EndHeight struct {
	EndHeight *EndHeight `protobuf:"bytes,4,opt,name=end_height,json=endHeight,proto3,oneof" json:"end_height,omitempty"`
}

func (*WALMessage_EventDataRoundState) isWALMessage_Sum() {}
func (*WALMessage_MsgInfo) isWALMessage_Sum()             {}
func (*WALMessage_TimeoutInfo) isWALMessage_Sum()         {}
func (*WALMessage_EndHeight) isWALMessage_Sum()           {}

func (m *WALMessage) GetSum() isWALMessage_Sum {
	if m != nil {
		return m.Sum
	}
	return nil
}

func (m *WALMessage) GetEventDataRoundState() *types.EventDataRoundState {
	if x, ok := m.GetSum().(*WALMessage_EventDataRoundState); ok {
		return x.EventDataRoundState
	}
	return nil
}

func (m *WALMessage) GetMsgInfo() *MsgInfo {
	if x, ok := m.GetSum().(*WALMessage_MsgInfo); ok {
		return x.MsgInfo
	}
	return nil
}

func (m *WALMessage) GetTimeoutInfo() *TimeoutInfo {
	if x, ok := m.GetSum().(*WALMessage_TimeoutInfo); ok {
		return x.TimeoutInfo
	}
	return nil
}

func (m *WALMessage) GetEndHeight() *EndHeight {
	if x, ok := m.GetSum().(*WALMessage_EndHeight); ok {
		return x.EndHeight
	}
	return nil
}

// XXX_OneofWrappers is for the internal use of the proto package.
func (*WALMessage) XXX_OneofWrappers() []interface{} {
	return []interface{}{
		(*WALMessage_EventDataRoundState)(nil),
		(*WALMessage_MsgInfo)(nil),
		(*WALMessage_TimeoutInfo)(nil),
		(*WALMessage_EndHeight)(nil),
	}
}

// TimedWALMessage wraps WALMessage and adds Time for debugging purposes.
type TimedWALMessage struct {
	Time                 time.Time   `protobuf:"bytes,1,opt,name=time,proto3,stdtime" json:"time"`
	Msg                  *WALMessage `protobuf:"bytes,2,opt,name=msg,proto3" json:"msg,omitempty"`
	XXX_NoUnkeyedLiteral struct{}    `json:"-"`
	XXX_unrecognized     []byte      `json:"-"`
	XXX_sizecache        int32       `json:"-"`
}

func (m *TimedWALMessage) Reset()         { *m = TimedWALMessage{} }
func (m *TimedWALMessage) String() string { return proto.CompactTextString(m) }
func (*TimedWALMessage) ProtoMessage()    {}
func (*TimedWALMessage) Descriptor() ([]byte, []int) {
	return fileDescriptor_60ad80fa14e37285, []int{4}
}
func (m *TimedWALMessage) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_TimedWALMessage.Unmarshal(m, b)
}
func (m *TimedWALMessage) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_TimedWALMessage.Marshal(b, m, deterministic)
}
func (m *TimedWALMessage) XXX_Merge(src proto.Message) {
	xxx_messageInfo_TimedWALMessage.Merge(m, src)
}
func (m *TimedWALMessage) XXX_Size() int {
	return xxx_messageInfo_TimedWALMessage.Size(m)
}
func (m *TimedWALMessage) XXX_DiscardUnknown() {
	xxx_messageInfo_TimedWALMessage.DiscardUnknown(m)
}

var xxx_messageInfo_TimedWALMessage proto.InternalMessageInfo

func (m *TimedWALMessage) GetTime() time.Time {
	if m != nil {
		return m.Time
	}
	return time.Time{}
}

func (m *TimedWALMessage) GetMsg() *WALMessage {
	if m != nil {
		return m.Msg
	}
	return nil
}

func init() {
	proto.RegisterType((*MsgInfo)(nil), "tendermint.proto.consensus.MsgInfo")
	proto.RegisterType((*TimeoutInfo)(nil), "tendermint.proto.consensus.TimeoutInfo")
	proto.RegisterType((*EndHeight)(nil), "tendermint.proto.consensus.EndHeight")
	proto.RegisterType((*WALMessage)(nil), "tendermint.proto.consensus.WALMessage")
	proto.RegisterType((*TimedWALMessage)(nil), "tendermint.proto.consensus.TimedWALMessage")
}

func init() { proto.RegisterFile("proto/consensus/walmsgs.proto", fileDescriptor_60ad80fa14e37285) }

var fileDescriptor_60ad80fa14e37285 = []byte{
	// 528 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x84, 0x53, 0xcd, 0x8a, 0x13, 0x41,
	0x10, 0xce, 0x6c, 0xb2, 0xf9, 0xa9, 0x28, 0xc2, 0x28, 0x4b, 0x1c, 0xd0, 0x84, 0x04, 0xd7, 0x80,
	0x30, 0x23, 0xeb, 0x65, 0xc1, 0x83, 0x1a, 0xb2, 0x92, 0xc0, 0x2e, 0x48, 0xbb, 0x20, 0x78, 0x19,
	0x26, 0x3b, 0x95, 0xce, 0xe0, 0x76, 0xf7, 0x30, 0x5d, 0xa3, 0xec, 0x03, 0x78, 0xdf, 0xa3, 0x8f,
	0xe4, 0xcd, 0x37, 0x58, 0xc1, 0x27, 0x91, 0xe9, 0x9e, 0xfc, 0x90, 0x60, 0xbc, 0x75, 0x57, 0xf5,
	0xf7, 0x7d, 0x55, 0xf5, 0x55, 0xc3, 0x93, 0x34, 0x53, 0xa4, 0x82, 0x2b, 0x25, 0x35, 0x4a, 0x9d,
	0xeb, 0xe0, 0x5b, 0x74, 0x2d, 0x34, 0xd7, 0xbe, 0x89, 0xbb, 0x1e, 0xa1, 0x8c, 0x31, 0x13, 0x89,
	0x24, 0x1b, 0xf1, 0x57, 0x2f, 0xbd, 0x63, 0x5a, 0x24, 0x59, 0x1c, 0xa6, 0x51, 0x46, 0x37, 0x81,
	0xa5, 0xe1, 0x8a, 0xab, 0xf5, 0xc9, 0x22, 0x3c, 0x6f, 0x5b, 0x62, 0xcd, 0xef, 0x75, 0x6c, 0x8e,
	0x6e, 0x52, 0xd4, 0x01, 0x7e, 0x45, 0x49, 0xcb, 0xcc, 0x53, 0xae, 0x14, 0xbf, 0x46, 0x4b, 0x3c,
	0xcb, 0xe7, 0x41, 0x9c, 0x67, 0x11, 0x25, 0x4a, 0x96, 0xf9, 0xee, 0x76, 0x9e, 0x12, 0x81, 0x9a,
	0x22, 0x91, 0xda, 0x07, 0xfd, 0x2f, 0xd0, 0xb8, 0xd0, 0x7c, 0x2a, 0xe7, 0xca, 0x7d, 0x0d, 0x55,
	0xa1, 0x79, 0xc7, 0xe9, 0x39, 0xc3, 0xf6, 0xc9, 0xc0, 0xff, 0x77, 0x4f, 0xfe, 0x05, 0x6a, 0x1d,
	0x71, 0x1c, 0xd5, 0x7e, 0xde, 0x75, 0x2b, 0xac, 0x40, 0xb9, 0x03, 0x68, 0xa4, 0x88, 0x59, 0x98,
	0xc4, 0x9d, 0x83, 0x9e, 0x33, 0x6c, 0x8d, 0xe0, 0xcf, 0x5d, 0xb7, 0xfe, 0x01, 0x31, 0x9b, 0x8e,
	0x59, 0xbd, 0x48, 0x4d, 0xe3, 0xfe, 0xad, 0x03, 0xed, 0xcb, 0x44, 0xa0, 0xca, 0xc9, 0x28, 0xbe,
	0x81, 0xe6, 0xb2, 0xde, 0x52, 0xf6, 0xb1, 0x6f, 0x0b, 0xf6, 0x97, 0x05, 0xfb, 0xe3, 0xf2, 0xc1,
	0xa8, 0x59, 0x88, 0xfd, 0xf8, 0xdd, 0x75, 0xd8, 0x0a, 0xe4, 0x1e, 0x41, 0x7d, 0x81, 0x09, 0x5f,
	0x90, 0x11, 0xad, 0xb2, 0xf2, 0xe6, 0x3e, 0x82, 0xc3, 0x4c, 0xe5, 0x32, 0xee, 0x54, 0x7b, 0xce,
	0xf0, 0x90, 0xd9, 0x8b, 0xeb, 0x42, 0x4d, 0x13, 0xa6, 0x9d, 0x5a, 0xcf, 0x19, 0xde, 0x67, 0xe6,
	0xdc, 0x1f, 0x40, 0xeb, 0x4c, 0xc6, 0x13, 0x0b, 0x5b, 0xd3, 0x39, 0x9b, 0x74, 0xfd, 0x5f, 0x07,
	0x00, 0x9f, 0xde, 0x9d, 0x97, 0x6d, 0xbb, 0x33, 0x38, 0x32, 0x26, 0x84, 0x71, 0x44, 0x51, 0x68,
	0xb8, 0x43, 0x4d, 0x11, 0x61, 0xd9, 0xc4, 0x8b, 0xdd, 0xd9, 0x19, 0xeb, 0xfc, 0xb3, 0x02, 0x35,
	0x8e, 0x28, 0x62, 0x05, 0xe6, 0x63, 0x01, 0x99, 0x54, 0xd8, 0x43, 0xdc, 0x0d, 0xbb, 0x6f, 0xa1,
	0x29, 0x34, 0x0f, 0x13, 0x39, 0x57, 0xa6, 0xb7, 0xff, 0x39, 0x62, 0x3d, 0x9c, 0x54, 0x58, 0x43,
	0x94, 0x76, 0x9e, 0xc3, 0x3d, 0xb2, 0xb3, 0xb6, 0x2c, 0x55, 0xc3, 0xf2, 0x7c, 0x1f, 0xcb, 0x86,
	0x37, 0x93, 0x0a, 0x6b, 0xd3, 0x86, 0x55, 0xef, 0x01, 0x50, 0xc6, 0x61, 0x39, 0x9e, 0x9a, 0xe1,
	0x7a, 0xb6, 0x8f, 0x6b, 0x35, 0xd5, 0x49, 0x85, 0xb5, 0x70, 0x79, 0x19, 0x1d, 0x42, 0x55, 0xe7,
	0xa2, 0xff, 0xdd, 0x81, 0x07, 0x85, 0x5a, 0xbc, 0x31, 0xd6, 0x53, 0xa8, 0x15, 0x8a, 0xe5, 0x10,
	0xbd, 0x9d, 0x4d, 0xb8, 0x5c, 0xae, 0xae, 0x5d, 0x85, 0xdb, 0x62, 0x15, 0x0c, 0xc2, 0x3d, 0xb5,
	0x9b, 0x6b, 0xe7, 0x74, 0xbc, 0xaf, 0xaa, 0xb5, 0x9c, 0x59, 0xdb, 0xd1, 0xc9, 0xe7, 0x97, 0x3c,
	0xa1, 0x45, 0x3e, 0xf3, 0xaf, 0x94, 0x08, 0xd6, 0xc0, 0xcd, 0xe3, 0xd6, 0xc7, 0x9c, 0xd5, 0x4d,
	0xe0, 0xd5, 0xdf, 0x00, 0x00, 0x00, 0xff, 0xff, 0x55, 0x7e, 0x02, 0x98, 0x15, 0x04, 0x00, 0x00,
}
