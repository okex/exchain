package types

import (
	"bytes"
	"errors"
	"fmt"
	"time"

	"github.com/tendermint/go-amino"

	"github.com/okex/exchain/libs/tendermint/crypto"
	tmbytes "github.com/okex/exchain/libs/tendermint/libs/bytes"
	tmproto "github.com/okex/exchain/libs/tendermint/proto/types"
)

const (
	// MaxVoteBytes is a maximum vote size (including amino overhead).
	MaxVoteBytes int64  = 223
	nilVoteStr   string = "nil-Vote"
)

var (
	ErrVoteUnexpectedStep            = errors.New("unexpected step")
	ErrVoteInvalidValidatorIndex     = errors.New("invalid validator index")
	ErrVoteInvalidValidatorAddress   = errors.New("invalid validator address")
	ErrVoteInvalidSignature          = errors.New("invalid signature")
	ErrVoteInvalidBlockHash          = errors.New("invalid block hash")
	ErrVoteNonDeterministicSignature = errors.New("non-deterministic signature")
	ErrVoteNil                       = errors.New("nil vote")
)

type ErrVoteConflictingVotes struct {
	*DuplicateVoteEvidence
}

func (err *ErrVoteConflictingVotes) Error() string {
	return fmt.Sprintf("Conflicting votes from validator %v", err.PubKey.Address())
}

func NewConflictingVoteError(val *Validator, vote1, vote2 *Vote) *ErrVoteConflictingVotes {
	return &ErrVoteConflictingVotes{
		NewDuplicateVoteEvidence(val.PubKey, vote1, vote2),
	}
}

// Address is hex bytes.
type Address = crypto.Address

// Vote represents a prevote, precommit, or commit vote from validators for
// consensus.
type Vote struct {
	Type             SignedMsgType `json:"type"`
	Height           int64         `json:"height"`
	Round            int           `json:"round"`
	BlockID          BlockID       `json:"block_id"` // zero if vote is nil.
	Timestamp        time.Time     `json:"timestamp"`
	ValidatorAddress Address       `json:"validator_address"`
	ValidatorIndex   int           `json:"validator_index"`
	Signature        []byte        `json:"signature"`
}

func (vote Vote) AminoSize(cdc *amino.Codec) int {
	var size = 0

	if vote.Type != 0 {
		size += 1 + amino.UvarintSize(uint64(vote.Type))
	}

	if vote.Height != 0 {
		size += 1 + amino.UvarintSize(uint64(vote.Height))
	}

	if vote.Round != 0 {
		size += 1 + amino.UvarintSize(uint64(vote.Round))
	}

	blockIDSize := vote.BlockID.AminoSize(cdc)
	if blockIDSize != 0 {
		size += 1 + amino.UvarintSize(uint64(blockIDSize)) + blockIDSize
	}

	timestampSize := amino.TimeSize(vote.Timestamp)
	if timestampSize != 0 {
		size += 1 + amino.UvarintSize(uint64(timestampSize)) + timestampSize
	}

	if len(vote.ValidatorAddress) != 0 {
		size += 1 + amino.ByteSliceSize(vote.ValidatorAddress)
	}

	if vote.ValidatorIndex != 0 {
		size += 1 + amino.UvarintSize(uint64(vote.ValidatorIndex))
	}

	if len(vote.Signature) != 0 {
		size += 1 + amino.ByteSliceSize(vote.Signature)
	}

	return size
}

func (vote *Vote) UnmarshalFromAmino(cdc *amino.Codec, data []byte) error {
	var dataLen uint64 = 0
	var subData []byte
	var timestampUpdated bool

	for {
		data = data[dataLen:]
		if len(data) == 0 {
			break
		}

		pos, pbType, err := amino.ParseProtoPosAndTypeMustOneByte(data[0])
		if err != nil {
			return err
		}
		data = data[1:]

		if pbType == amino.Typ3_ByteLength {
			var n int
			dataLen, n, err = amino.DecodeUvarint(data)
			if err != nil {
				return err
			}
			data = data[n:]
			if len(data) < int(dataLen) {
				return fmt.Errorf("invalid data len")
			}
			subData = data[:dataLen]
		}

		switch pos {
		case 1:
			u64, n, err := amino.DecodeUvarint(data)
			if err != nil {
				return err
			}
			vote.Type = SignedMsgType(u64)
			dataLen = uint64(n)
		case 2:
			u64, n, err := amino.DecodeUvarint(data)
			if err != nil {
				return err
			}
			vote.Height = int64(u64)
			dataLen = uint64(n)
		case 3:
			u64, n, err := amino.DecodeUvarint(data)
			if err != nil {
				return err
			}
			vote.Round = int(u64)
			dataLen = uint64(n)
		case 4:
			err = vote.BlockID.UnmarshalFromAmino(cdc, subData)
			if err != nil {
				return err
			}
		case 5:
			vote.Timestamp, _, err = amino.DecodeTime(subData)
			if err != nil {
				return err
			}
			timestampUpdated = true
		case 6:
			vote.ValidatorAddress = make([]byte, len(subData))
			copy(vote.ValidatorAddress, subData)
		case 7:
			u64, n, err := amino.DecodeUvarint(data)
			if err != nil {
				return err
			}
			vote.ValidatorIndex = int(u64)
			dataLen = uint64(n)
		case 8:
			vote.Signature = make([]byte, len(subData))
			copy(vote.Signature, subData)
		default:
			return fmt.Errorf("unexpect feild num %d", pos)
		}
	}
	if !timestampUpdated {
		vote.Timestamp = amino.ZeroTime
	}
	return nil
}

// CommitSig converts the Vote to a CommitSig.
func (vote *Vote) CommitSig() CommitSig {
	if vote == nil {
		return NewCommitSigAbsent()
	}

	var blockIDFlag BlockIDFlag
	switch {
	case vote.BlockID.IsComplete():
		blockIDFlag = BlockIDFlagCommit
	case vote.BlockID.IsZero():
		blockIDFlag = BlockIDFlagNil
	default:
		panic(fmt.Sprintf("Invalid vote %v - expected BlockID to be either empty or complete", vote))
	}

	return CommitSig{
		BlockIDFlag:      blockIDFlag,
		ValidatorAddress: vote.ValidatorAddress,
		Timestamp:        vote.Timestamp,
		Signature:        vote.Signature,
	}
}

func (vote *Vote) SignBytes(chainID string) []byte {
	if HigherThanVenus1(vote.Height) {
		return vote.ibcSignBytes(chainID)
	}
	return vote.originSignBytes(chainID)
}

func (vote *Vote) ibcSignBytes(chainID string) []byte {
	return VoteSignBytes(chainID, vote)
}
func (vote *Vote) originSignBytes(chainId string) []byte {
	bz, err := cdc.MarshalBinaryLengthPrefixed(CanonicalizeVote(chainId, vote))
	if err != nil {
		panic(err)
	}
	return bz
}

func (vote *Vote) Copy() *Vote {
	voteCopy := *vote
	return &voteCopy
}

func (vote *Vote) String() string {
	if vote == nil {
		return nilVoteStr
	}

	var typeString string
	switch vote.Type {
	case PrevoteType:
		typeString = "Prevote"
	case PrecommitType:
		typeString = "Precommit"
	default:
		panic("Unknown vote type")
	}

	return fmt.Sprintf("Vote{%v:%X %v/%02d/%v(%v) %X %X @ %s}",
		vote.ValidatorIndex,
		tmbytes.Fingerprint(vote.ValidatorAddress),
		vote.Height,
		vote.Round,
		vote.Type,
		typeString,
		tmbytes.Fingerprint(vote.BlockID.Hash),
		tmbytes.Fingerprint(vote.Signature),
		CanonicalTime(vote.Timestamp),
	)
}

func (vote *Vote) Verify(chainID string, pubKey crypto.PubKey) error {
	if !bytes.Equal(pubKey.Address(), vote.ValidatorAddress) {
		return ErrVoteInvalidValidatorAddress
	}

	if !pubKey.VerifyBytes(vote.SignBytes(chainID), vote.Signature) {
		return ErrVoteInvalidSignature
	}
	return nil
}

// ValidateBasic performs basic validation.
func (vote *Vote) ValidateBasic() error {
	if !IsVoteTypeValid(vote.Type) {
		return errors.New("invalid Type")
	}
	if vote.Height < 0 {
		return errors.New("negative Height")
	}
	if vote.Round < 0 {
		return errors.New("negative Round")
	}

	// NOTE: Timestamp validation is subtle and handled elsewhere.

	if err := vote.BlockID.ValidateBasic(); err != nil {
		return fmt.Errorf("wrong BlockID: %v", err)
	}
	// BlockID.ValidateBasic would not err if we for instance have an empty hash but a
	// non-empty PartsSetHeader:
	if !vote.BlockID.IsZero() && !vote.BlockID.IsComplete() {
		return fmt.Errorf("blockID must be either empty or complete, got: %v", vote.BlockID)
	}
	if len(vote.ValidatorAddress) != crypto.AddressSize {
		return fmt.Errorf("expected ValidatorAddress size to be %d bytes, got %d bytes",
			crypto.AddressSize,
			len(vote.ValidatorAddress),
		)
	}
	if vote.ValidatorIndex < 0 {
		return errors.New("negative ValidatorIndex")
	}
	if len(vote.Signature) == 0 {
		return errors.New("signature is missing")
	}
	if len(vote.Signature) > MaxSignatureSize {
		return fmt.Errorf("signature is too big (max: %d)", MaxSignatureSize)
	}
	return nil
}

// ToProto converts the handwritten type to proto generated type
// return type, nil if everything converts safely, otherwise nil, error
func (vote *Vote) ToProto() *tmproto.Vote {
	if vote == nil {
		return nil
	}

	return &tmproto.Vote{
		Type:             tmproto.SignedMsgType(vote.Type),
		Height:           vote.Height,
		Round:            int64(vote.Round),
		BlockID:          vote.BlockID.ToProto(),
		Timestamp:        vote.Timestamp,
		ValidatorAddress: vote.ValidatorAddress,
		ValidatorIndex:   int64(vote.ValidatorIndex),
		Signature:        vote.Signature,
	}
}

// FromProto converts a proto generetad type to a handwritten type
// return type, nil if everything converts safely, otherwise nil, error
func VoteFromProto(pv *tmproto.Vote) (*Vote, error) {
	if pv == nil {
		return nil, errors.New("nil vote")
	}

	blockID, err := BlockIDFromProto(&pv.BlockID)
	if err != nil {
		return nil, err
	}

	vote := new(Vote)
	vote.Type = SignedMsgType(pv.Type)
	vote.Height = pv.Height
	vote.Round = int(pv.Round)
	vote.BlockID = *blockID
	vote.Timestamp = pv.Timestamp
	vote.ValidatorAddress = pv.ValidatorAddress
	vote.ValidatorIndex = int(pv.ValidatorIndex)
	vote.Signature = pv.Signature

	return vote, vote.ValidateBasic()
}
