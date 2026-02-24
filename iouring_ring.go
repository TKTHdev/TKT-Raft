package raft

// Minimal io_uring wrapper using raw syscalls.
//
// Architecture:
//   SQ ring (submission queue):
//     array[i] = i (initialized once, natural mapping)
//     tail is advanced by user when adding SQEs
//     head is advanced by kernel when consuming SQEs
//   SQE array (at IORING_OFF_SQES):
//     64-byte entries, indexed by sq.array values
//   CQ ring (completion queue, shares mmap with SQ if SINGLE_MMAP):
//     cqes[i] = 16-byte completion entries
//     head is advanced by user after reading CQEs
//     tail is advanced by kernel when posting completions

import (
	"fmt"
	"sync/atomic"
	"syscall"
	"unsafe"
)

const (
	sysIoUringSetup uintptr = 425
	sysIoUringEnter uintptr = 426

	iouringOpFsync = 3  // IORING_OP_FSYNC
	iouringOpWrite = 23 // IORING_OP_WRITE (scatter/gather with offset)

	iouringFsyncDatasync  = 1 // IORING_FSYNC_DATASYNC
	iosqeIOLink           = 4 // IOSQE_IO_LINK  (1 << 2)
	iouringEnterGetEvents = 1 // IORING_ENTER_GETEVENTS

	iouringFeatSingleMmap = 1 // IORING_FEAT_SINGLE_MMAP

	iouringOffSqRing uintptr = 0
	iouringOffCqRing uintptr = 0x8000000
	iouringOffSqes   uintptr = 0x10000000

	sqeSize uintptr = 64
	cqeSize uintptr = 16
)

// ioUringParams mirrors struct io_uring_params (120 bytes).
type ioUringParams struct {
	sqEntries    uint32
	cqEntries    uint32
	flags        uint32
	sqThreadCPU  uint32
	sqThreadIdle uint32
	features     uint32
	wqFD         uint32
	resv         [3]uint32
	sqOff        ioSqRingOffsets // 40 bytes
	cqOff        ioCqRingOffsets // 40 bytes
}

// ioSqRingOffsets mirrors struct io_sqring_offsets (40 bytes).
type ioSqRingOffsets struct {
	head        uint32
	tail        uint32
	ringMask    uint32
	ringEntries uint32
	flags       uint32
	dropped     uint32
	array       uint32
	_resv1      uint32
	_userAddr   uint64
}

// ioCqRingOffsets mirrors struct io_cqring_offsets (40 bytes).
type ioCqRingOffsets struct {
	head        uint32
	tail        uint32
	ringMask    uint32
	ringEntries uint32
	overflow    uint32
	cqes        uint32
	flags       uint32
	_resv1      uint32
	_userAddr   uint64
}

// ioUringSQE mirrors struct io_uring_sqe (64 bytes).
type ioUringSQE struct {
	opcode      uint8
	flags       uint8
	ioprio      uint16
	fd          int32
	off         uint64 // file offset for IORING_OP_WRITE
	addr        uint64 // buffer address
	len         uint32 // buffer length
	rwFlags     uint32 // rw_flags or fsync_flags
	userData    uint64
	bufIndex    uint16
	personality uint16
	spliceFdIn  int32
	addr3       uint64
	_pad2       uint64
}

// ioUringCQE mirrors struct io_uring_cqe (16 bytes).
type ioUringCQE struct {
	userData uint64
	res      int32
	flags    uint32
}

// ioRing holds the mmap'd io_uring ring structures.
// All submit/reap operations must be called from a single goroutine.
type ioRing struct {
	fd       int
	ringSize uint32

	ringMmap []byte // SQ ring (and CQ if SINGLE_MMAP)
	sqesMmap []byte // SQE array
	cqMmap   []byte // CQ ring (non-nil only when not SINGLE_MMAP)

	// Pointers into mmap'd memory (not heap, GC-safe)
	sqHead  *uint32
	sqTail  *uint32
	sqMask  *uint32
	sqArray unsafe.Pointer // []uint32 mapping ring slots → SQE indices

	sqes unsafe.Pointer // []ioUringSQE

	cqHead *uint32
	cqTail *uint32
	cqMask *uint32
	cqes   unsafe.Pointer // []ioUringCQE
}

func newIoRing(entries uint32) (*ioRing, error) {
	var params ioUringParams
	fd, _, errno := syscall.Syscall(
		sysIoUringSetup,
		uintptr(entries),
		uintptr(unsafe.Pointer(&params)),
		0,
	)
	if errno != 0 {
		return nil, fmt.Errorf("io_uring_setup: %w", errno)
	}
	ringFd := int(fd)

	r := &ioRing{fd: ringFd, ringSize: params.sqEntries}

	// Compute mmap sizes.
	sqRingSize := uintptr(params.sqOff.array) + uintptr(params.sqEntries)*4
	cqRingSize := uintptr(params.cqOff.cqes) + uintptr(params.cqEntries)*cqeSize
	ringMmapSize := sqRingSize
	if cqRingSize > ringMmapSize {
		ringMmapSize = cqRingSize
	}
	sqesMmapSize := uintptr(params.sqEntries) * sqeSize

	// mmap SQ ring (contains CQ ring data too if SINGLE_MMAP).
	ringData, err := syscall.Mmap(
		ringFd, int64(iouringOffSqRing), int(ringMmapSize),
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_SHARED|syscall.MAP_POPULATE,
	)
	if err != nil {
		syscall.Close(ringFd)
		return nil, fmt.Errorf("mmap sq ring: %w", err)
	}
	r.ringMmap = ringData

	// mmap SQE array (always separate).
	sqesData, err := syscall.Mmap(
		ringFd, int64(iouringOffSqes), int(sqesMmapSize),
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_SHARED|syscall.MAP_POPULATE,
	)
	if err != nil {
		syscall.Munmap(ringData)
		syscall.Close(ringFd)
		return nil, fmt.Errorf("mmap sqes: %w", err)
	}
	r.sqesMmap = sqesData

	// mmap CQ ring separately if not SINGLE_MMAP.
	if params.features&iouringFeatSingleMmap == 0 {
		cqData, err := syscall.Mmap(
			ringFd, int64(iouringOffCqRing), int(cqRingSize),
			syscall.PROT_READ|syscall.PROT_WRITE,
			syscall.MAP_SHARED|syscall.MAP_POPULATE,
		)
		if err != nil {
			syscall.Munmap(sqesData)
			syscall.Munmap(ringData)
			syscall.Close(ringFd)
			return nil, fmt.Errorf("mmap cq ring: %w", err)
		}
		r.cqMmap = cqData
	}

	// Wire up SQ ring pointers.
	sqBase := unsafe.Pointer(&ringData[0])
	r.sqHead  = (*uint32)(unsafe.Pointer(uintptr(sqBase) + uintptr(params.sqOff.head)))
	r.sqTail  = (*uint32)(unsafe.Pointer(uintptr(sqBase) + uintptr(params.sqOff.tail)))
	r.sqMask  = (*uint32)(unsafe.Pointer(uintptr(sqBase) + uintptr(params.sqOff.ringMask)))
	r.sqArray = unsafe.Pointer(uintptr(sqBase) + uintptr(params.sqOff.array))

	// Wire up SQE array pointer.
	r.sqes = unsafe.Pointer(&sqesData[0])

	// Initialize sq.array[i] = i (natural 1-to-1 mapping of ring slots to SQEs).
	for i := uint32(0); i < params.sqEntries; i++ {
		*(*uint32)(unsafe.Pointer(uintptr(r.sqArray) + uintptr(i*4))) = i
	}

	// Wire up CQ ring pointers.
	var cqBase unsafe.Pointer
	if r.cqMmap != nil {
		cqBase = unsafe.Pointer(&r.cqMmap[0])
	} else {
		cqBase = sqBase
	}
	r.cqHead = (*uint32)(unsafe.Pointer(uintptr(cqBase) + uintptr(params.cqOff.head)))
	r.cqTail = (*uint32)(unsafe.Pointer(uintptr(cqBase) + uintptr(params.cqOff.tail)))
	r.cqMask = (*uint32)(unsafe.Pointer(uintptr(cqBase) + uintptr(params.cqOff.ringMask)))
	r.cqes   = unsafe.Pointer(uintptr(cqBase) + uintptr(params.cqOff.cqes))

	return r, nil
}

// submitWriteSync submits Pwrite + Fdatasync as a linked pair and waits
// for both to complete. buf must remain valid until this returns.
func (r *ioRing) submitWriteSync(fd int, buf []byte, offset int64) error {
	tail := atomic.LoadUint32(r.sqTail)
	mask := atomic.LoadUint32(r.sqMask)

	// SQE 0: Write, linked to SQE 1.
	sqe0 := r.sqeAt(tail & mask)
	*sqe0 = ioUringSQE{}
	sqe0.opcode   = iouringOpWrite
	sqe0.flags    = iosqeIOLink
	sqe0.fd       = int32(fd)
	sqe0.off      = uint64(offset)
	sqe0.addr     = uint64(uintptr(unsafe.Pointer(&buf[0])))
	sqe0.len      = uint32(len(buf))
	sqe0.userData = 1

	// SQE 1: Fdatasync (tail of the linked chain, no LINK flag).
	sqe1 := r.sqeAt((tail + 1) & mask)
	*sqe1 = ioUringSQE{}
	sqe1.opcode   = iouringOpFsync
	sqe1.fd       = int32(fd)
	sqe1.rwFlags  = iouringFsyncDatasync
	sqe1.userData = 2

	atomic.StoreUint32(r.sqTail, tail+2)
	return r.enterAndReap(2, 2)
}

// submitWriteAsync submits only Pwrite (no fsync) and waits for completion.
// buf must remain valid until this returns.
func (r *ioRing) submitWriteAsync(fd int, buf []byte, offset int64) error {
	tail := atomic.LoadUint32(r.sqTail)
	mask := atomic.LoadUint32(r.sqMask)

	sqe := r.sqeAt(tail & mask)
	*sqe = ioUringSQE{}
	sqe.opcode   = iouringOpWrite
	sqe.fd       = int32(fd)
	sqe.off      = uint64(offset)
	sqe.addr     = uint64(uintptr(unsafe.Pointer(&buf[0])))
	sqe.len      = uint32(len(buf))
	sqe.userData = 1

	atomic.StoreUint32(r.sqTail, tail+1)
	return r.enterAndReap(1, 1)
}

func (r *ioRing) sqeAt(index uint32) *ioUringSQE {
	return (*ioUringSQE)(unsafe.Pointer(uintptr(r.sqes) + uintptr(index)*sqeSize))
}

func (r *ioRing) cqeAt(index uint32) *ioUringCQE {
	return (*ioUringCQE)(unsafe.Pointer(uintptr(r.cqes) + uintptr(index)*cqeSize))
}

// enterAndReap submits toSubmit SQEs, waits for toWait completions, then
// reaps and checks results.
func (r *ioRing) enterAndReap(toSubmit, toWait uint32) error {
	_, _, errno := syscall.Syscall6(
		sysIoUringEnter,
		uintptr(r.fd),
		uintptr(toSubmit),
		uintptr(toWait),
		uintptr(iouringEnterGetEvents),
		0, 0,
	)
	if errno != 0 {
		return fmt.Errorf("io_uring_enter: %w", errno)
	}

	head := atomic.LoadUint32(r.cqHead)
	mask := atomic.LoadUint32(r.cqMask)

	var firstErr error
	for i := uint32(0); i < toWait; i++ {
		cqe := r.cqeAt((head + i) & mask)
		if cqe.res < 0 && firstErr == nil {
			firstErr = syscall.Errno(-cqe.res)
		}
	}
	atomic.StoreUint32(r.cqHead, head+toWait)
	return firstErr
}

func (r *ioRing) close() {
	if r.cqMmap != nil {
		syscall.Munmap(r.cqMmap)
	}
	syscall.Munmap(r.sqesMmap)
	syscall.Munmap(r.ringMmap)
	syscall.Close(r.fd)
}
