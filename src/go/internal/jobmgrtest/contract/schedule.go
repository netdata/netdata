package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

type PreparedOperation struct {
	Sequence         int
	Class            PerformanceClass
	Key              string
	UID              string
	Request          []byte
	Followup         []byte
	RequestSHA256    string
	FollowupSHA256   string
	UsefulWorkSHA256 string
}

type PerformanceSchedule struct {
	WorkloadID string
	PairNonce  [16]byte
	Operations []PreparedOperation
	ByUID      map[string]int
	SHA256     string
}

func BuildPerformanceSchedule(workloadID string, pairNonce [16]byte) (*PerformanceSchedule, error) {
	workload, err := performanceWorkloadByID(workloadID)
	if err != nil {
		return nil, err
	}
	schedule := &PerformanceSchedule{
		WorkloadID: workloadID,
		PairNonce:  pairNonce,
		Operations: make([]PreparedOperation, PerformanceOperations),
		ByUID:      make(map[string]int, PerformanceOperations),
	}
	digest := sha256.New()
	for sequence := range schedule.Operations {
		class, key, err := workload.ClassAndKey(sequence)
		if err != nil {
			return nil, err
		}
		uid := PerformanceUID(pairNonce, uint32(sequence))
		request, err := PerformanceRequest(class, uid, key)
		if err != nil {
			return nil, err
		}
		followup, err := PerformanceFollowup(class, uid)
		if err != nil {
			return nil, err
		}
		usefulWork, err := UsefulWorkSHA256(class)
		if err != nil {
			return nil, err
		}
		if _, exists := schedule.ByUID[uid]; exists {
			return nil, fmt.Errorf("performance schedule: duplicate UID %s", uid)
		}
		schedule.ByUID[uid] = sequence
		schedule.Operations[sequence] = PreparedOperation{
			Sequence:         sequence,
			Class:            class,
			Key:              key,
			UID:              uid,
			Request:          request,
			Followup:         followup,
			RequestSHA256:    sha256Hex(request),
			FollowupSHA256:   sha256Hex(followup),
			UsefulWorkSHA256: usefulWork,
		}
		_, _ = digest.Write([]byte{byte(class)})
		_, _ = digest.Write([]byte(key))
		_, _ = digest.Write([]byte(uid))
		_, _ = digest.Write(request)
		_, _ = digest.Write(followup)
		_, _ = digest.Write([]byte(usefulWork))
	}
	schedule.SHA256 = hex.EncodeToString(digest.Sum(nil))
	return schedule, nil
}

func (schedule *PerformanceSchedule) Validate(workloadID string, pairNonce [16]byte) error {
	if schedule == nil {
		return errors.New("performance schedule: nil schedule")
	}
	if schedule.WorkloadID != workloadID || schedule.PairNonce != pairNonce {
		return errors.New("performance schedule: coordinate differs")
	}
	if len(schedule.Operations) != PerformanceOperations || len(schedule.ByUID) != PerformanceOperations {
		return errors.New("performance schedule: incomplete operation set")
	}
	if len(schedule.SHA256) != sha256.Size*2 {
		return errors.New("performance schedule: invalid digest")
	}
	for sequence, operation := range schedule.Operations {
		if operation.Sequence != sequence || schedule.ByUID[operation.UID] != sequence {
			return errors.New("performance schedule: sequence index differs")
		}
	}
	return nil
}

func performanceWorkloadByID(id string) (PerformanceWorkload, error) {
	for _, workload := range PerformanceWorkloads() {
		if workload.ID == id {
			return workload, nil
		}
	}
	return PerformanceWorkload{}, fmt.Errorf("performance schedule: unknown workload %q", id)
}
