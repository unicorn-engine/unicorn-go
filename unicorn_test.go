package unicorn

import (
	"log"
	"os"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const INVALID_PLACEHOLDER = 0x1312

const GENERIC_PLACEHOLDER = 0x1914

const alphabet = "abcdefghijklmnopqrstuvwxyz"

const (
	GENERIC_MAPPING_ADDR   = 0x1000
	GENERIC_MAPPING_SIZE   = 0x1000
	GENERIC_MAPPING_PROT   = PROT_ALL
	GENERIC_REGISTER_VALUE = GENERIC_PLACEHOLDER
	GENERIC_CODE_ADDR      = 0x1000
	GENERIC_CODE_SIZE      = 0x1000
)

type code []byte

// x86_64: NOP; NOP; NOP; HLT
var X86_CODE code = []byte{0x90, 0x90, 0x90, 0xF4}

// x86_64: INC RAX (REX.W + FF /0) ; HLT
// REX.W prefix: 0x48, INC RAX: 0xFF 0xC0, HLT: 0xF4
var X86_INC_RAX code = []byte{0x48, 0xFF, 0xC0, 0xF4}

// x86_64: JMP $
// Real instruction is JMP -2
var X86_INFINITE_LOOP code = []byte{0xEB, 0xFE}

func GetGenericMu(t *testing.T) Unicorn {
	t.Helper()
	mu, err := NewUnicorn(ARCH_X86, MODE_64)
	require.NoError(t, err)
	return mu
}

func GetGenericMapping(t *testing.T, mu Unicorn) {
	t.Helper()
	err := mu.MemMapProt(GENERIC_MAPPING_ADDR, GENERIC_MAPPING_SIZE, GENERIC_MAPPING_PROT)
	require.NoError(t, err)
}

func CleanGenericMapping(t *testing.T, mu Unicorn) {
	t.Helper()
	err := mu.MemUnmap(GENERIC_MAPPING_ADDR, GENERIC_MAPPING_SIZE)
	require.NoError(t, err)
}

func GenerateCyclic(l int) []byte {
	var padding []byte
	for i := range l {
		if (i+1)%4 == 0 {
			padding = append(padding, alphabet[i%len(alphabet)])
		} else {
			padding = append(padding, []byte("a")...)
		}
	}
	return padding
}

func GetGenericMemoryPadding(t *testing.T, mu Unicorn) {
	t.Helper()
	paddingBuffer := GenerateCyclic(GENERIC_MAPPING_SIZE)
	err := mu.MemWrite(GENERIC_MAPPING_ADDR, paddingBuffer)
	require.NoError(t, err)
}

func GetGenericRegisters(t *testing.T, mu Unicorn) {
	t.Helper()
	err := mu.RegWrite(X86_REG_RAX, GENERIC_REGISTER_VALUE)
	require.NoError(t, err)
	err = mu.RegWrite(X86_REG_RBX, GENERIC_REGISTER_VALUE)
	require.NoError(t, err)
}

func GetGenericCode(t *testing.T, mu Unicorn, c code) {
	t.Helper()
	GetGenericMapping(t, mu)
	err := mu.MemWrite(GENERIC_CODE_ADDR, c)
	require.NoError(t, err)
}

func TestNewUnicorn(t *testing.T) {
	t.Run("succeeds with compatible arch and mode", func(t *testing.T) {
		mu, err := NewUnicorn(ARCH_X86, MODE_64)
		assert.NoError(t, err)
		assert.NotNil(t, mu)
		mu.Close()
	})

	t.Run("fails with incompatible arch and mode", func(t *testing.T) {
		mu, err := NewUnicorn(ARCH_MIPS, MODE_16)
		assert.Equal(t, err, UcError(ERR_MODE))
		assert.Nil(t, mu)
	})

	t.Run("fails with invalid arch", func(t *testing.T) {
		mu, err := NewUnicorn(INVALID_PLACEHOLDER, MODE_16)
		assert.Equal(t, err, UcError(ERR_ARCH))
		assert.Nil(t, mu)
	})
}

func TestClose(t *testing.T) {
	t.Run("succeeds on first call", func(t *testing.T) {
		mu, _ := NewUnicorn(ARCH_X86, MODE_64)

		err := mu.Close()
		assert.NoError(t, err)
	})

	t.Run("is idempotent on second call", func(t *testing.T) {
		mu, _ := NewUnicorn(ARCH_X86, MODE_64)

		err := mu.Close()
		assert.NoError(t, err)

		err = mu.Close()
		assert.NoError(t, err)
	})
}

// // Concurency not handled by unicorn-go yet
// func TestConcurence(t *testing.T) {
// 	t.Run("concurrent access", func(t *testing.T) {
// 		mu, _ := NewUnicorn(ARCH_X86, MODE_64)
//
// 		var wg sync.WaitGroup
// 		for i := 0; i < 10; i++ {
// 			wg.Add(1)
// 			go func() {
// 				defer wg.Done()
// 				mu.MemMap(uint64(0x1000*i), 0x1000)
// 			}()
// 		}
// 		wg.Wait()
// 	})
// }

func TestMemMap(t *testing.T) {
	t.Run("succeeds with valid address and size", func(t *testing.T) {
		mu := GetGenericMu(t)

		err := mu.MemMap(0x1000, 0x1000)
		assert.NoError(t, err)
	})

	t.Run("fails with invalid size", func(t *testing.T) {
		mu := GetGenericMu(t)

		err := mu.MemMap(0x1000, INVALID_PLACEHOLDER)
		assert.Equal(t, err, UcError(ERR_ARG))
	})
}

func TestMemMapProt(t *testing.T) {
	t.Run("succeeds with read-execute permissions", func(t *testing.T) {
		mu := GetGenericMu(t)

		err := mu.MemMapProt(0x1000, 0x1000, PROT_EXEC|PROT_READ)
		assert.NoError(t, err)
	})

	t.Run("fails with invalid protection flags", func(t *testing.T) {
		mu := GetGenericMu(t)

		err := mu.MemMapProt(0x1000, 0x1000, INVALID_PLACEHOLDER)
		assert.Equal(t, err, UcError(ERR_ARG))
	})
}

func TestMemMapPtr(t *testing.T) {
	t.Run("succeeds with valid mmap'd region", func(t *testing.T) {
		mu := GetGenericMu(t)

		fd, err := os.OpenFile("/proc/self/exe", os.O_RDONLY, 0777)
		if err != nil {
			log.Fatalln("Error in OpenFile()", err)
		}

		buf, err := syscall.Mmap(
			int(fd.Fd()), 0,
			0x1000, syscall.PROT_WRITE|syscall.PROT_READ,
			syscall.MAP_PRIVATE,
		)
		if err != nil {
			log.Fatalln("Error in Mmap()", err)
		}

		err = mu.MemMapPtr(0x1000, 0x1000, PROT_EXEC|PROT_READ, unsafe.Pointer(&buf))
		assert.NoError(t, err)

		if syscall.Munmap(buf); err != nil {
			log.Fatalln("Error in Munmap()", err)
		}
	})

	t.Run("succeeds with arbitrary pointer", func(t *testing.T) {
		mu := GetGenericMu(t)

		err := mu.MemMapPtr(
			0x1000,
			0x1000,
			PROT_EXEC|PROT_READ,
			unsafe.Pointer(uintptr(INVALID_PLACEHOLDER)),
		)

		assert.NoError(t, err)
	})

	t.Run("fails with invalid size on valid mmap'd region", func(t *testing.T) {
		mu := GetGenericMu(t)

		fd, err := os.OpenFile("/proc/self/exe", os.O_RDONLY, 0777)
		if err != nil {
			log.Fatalln("Error in OpenFile()", err)
		}

		buf, err := syscall.Mmap(
			int(fd.Fd()), 0,
			0x1000, syscall.PROT_WRITE|syscall.PROT_READ,
			syscall.MAP_PRIVATE,
		)
		if err != nil {
			log.Fatalln("Error in Mmap()", err)
		}

		err = mu.MemMapPtr(0x1000, INVALID_PLACEHOLDER, PROT_EXEC|PROT_READ, unsafe.Pointer(&buf))
		assert.Equal(t, err, UcError(ERR_ARG))

		if err := syscall.Munmap(buf); err != nil {
			log.Fatalln("Error in Munmap()", err)
		}
	})
}

func TestMemProtect(t *testing.T) {
	t.Run("succeeds with valid protection flags", func(t *testing.T) {
		mu := GetGenericMu(t)
		GetGenericMapping(t, mu)

		err := mu.MemProtect(GENERIC_MAPPING_ADDR, GENERIC_MAPPING_SIZE, PROT_READ|PROT_EXEC)
		assert.NoError(t, err)
	})

	t.Run("fails with invalid protection flags", func(t *testing.T) {
		mu := GetGenericMu(t)
		GetGenericMapping(t, mu)

		err := mu.MemProtect(GENERIC_MAPPING_ADDR, GENERIC_MAPPING_SIZE, INVALID_PLACEHOLDER)
		assert.Equal(t, err, UcError(ERR_ARG))
	})

	t.Run("fails on unmapped address", func(t *testing.T) {
		mu := GetGenericMu(t)
		GetGenericMapping(t, mu)

		err := mu.MemProtect(0x2000, GENERIC_MAPPING_SIZE, PROT_ALL)
		assert.Equal(t, err, UcError(ERR_NOMEM))
	})
}

func TestMemUnmap(t *testing.T) {
	t.Run("succeeds on mapped region", func(t *testing.T) {
		mu := GetGenericMu(t)
		GetGenericMapping(t, mu)

		err := mu.MemUnmap(GENERIC_MAPPING_ADDR, GENERIC_MAPPING_SIZE)
		assert.NoError(t, err)
	})

	t.Run("fails on unmapped address", func(t *testing.T) {
		mu := GetGenericMu(t)
		GetGenericMapping(t, mu)

		err := mu.MemUnmap(0x2000, GENERIC_MAPPING_SIZE)
		assert.Equal(t, err, UcError(ERR_NOMEM))
	})

	t.Run("fails with invalid size", func(t *testing.T) {
		mu := GetGenericMu(t)
		GetGenericMapping(t, mu)

		err := mu.MemUnmap(GENERIC_MAPPING_ADDR, INVALID_PLACEHOLDER)
		assert.Equal(t, err, UcError(ERR_ARG))
	})

	t.Run("fails on already unmapped region", func(t *testing.T) {
		mu := GetGenericMu(t)
		GetGenericMapping(t, mu)

		err := mu.MemUnmap(GENERIC_MAPPING_ADDR, GENERIC_MAPPING_SIZE)
		assert.NoError(t, err)

		err = mu.MemUnmap(GENERIC_MAPPING_ADDR, GENERIC_MAPPING_SIZE)
		assert.Error(t, err, UcError(ERR_NOMEM))
	})
}

func TestMemRegions(t *testing.T) {
	t.Run("returns mapped region with correct fields", func(t *testing.T) {
		mu := GetGenericMu(t)
		GetGenericMapping(t, mu)
		want := MemRegion{
			Begin: GENERIC_MAPPING_ADDR,
			End:   GENERIC_MAPPING_ADDR + GENERIC_MAPPING_SIZE - 1,
			Prot:  PROT_ALL,
		}
		wantedNumberOfRegions := 1

		regions, err := mu.MemRegions()

		assert.NoError(t, err)
		assert.Equal(t, *regions[0], want)
		assert.Equal(t, len(regions), wantedNumberOfRegions)
	})

	t.Run("returns empty slice with no mappings", func(t *testing.T) {
		mu := GetGenericMu(t)
		want := []*MemRegion{}
		wantedNumberOfRegions := 0

		regions, err := mu.MemRegions()

		assert.NoError(t, err)
		assert.Equal(t, regions, want)
		assert.Equal(t, len(regions), wantedNumberOfRegions)
	})

	t.Run("reflects updated layout after unmap", func(t *testing.T) {
		mu := GetGenericMu(t)
		GetGenericMapping(t, mu)
		want := MemRegion{
			Begin: GENERIC_MAPPING_ADDR,
			End:   GENERIC_MAPPING_ADDR + GENERIC_MAPPING_SIZE - 1,
			Prot:  PROT_ALL,
		}
		wantedNumberOfRegions := 1

		regions, err := mu.MemRegions()

		assert.NoError(t, err)
		assert.Equal(t, *regions[0], want)
		assert.Equal(t, len(regions), wantedNumberOfRegions)

		CleanGenericMapping(t, mu)

		regions, err = mu.MemRegions()
		assert.NoError(t, err)
	})
}

func TestMemRead(t *testing.T) {
	t.Run("returns written data on mapped address", func(t *testing.T) {
		mu := GetGenericMu(t)
		GetGenericMapping(t, mu)
		GetGenericMemoryPadding(t, mu)
		memory, err := mu.MemRead(GENERIC_MAPPING_ADDR, 0x100)

		assert.Equal(t, memory, GenerateCyclic(0x100))
		assert.NoError(t, err)
	})

	t.Run("fails on unmapped address", func(t *testing.T) {
		mu := GetGenericMu(t)
		memory, err := mu.MemRead(GENERIC_MAPPING_ADDR, 0x100)

		assert.Equal(t, memory, make([]byte, 0x100))
		assert.Equal(t, err, UcError(ERR_READ_UNMAPPED))
	})
}

func TestMemWrite(t *testing.T) {
	t.Run("persists data readable by MemRead", func(t *testing.T) {
		mu := GetGenericMu(t)
		GetGenericMapping(t, mu)
		want := GenerateCyclic(0x100)

		err := mu.MemWrite(GENERIC_MAPPING_ADDR, want)
		assert.NoError(t, err)

		memory, err := mu.MemRead(GENERIC_MAPPING_ADDR, 0x100)
		assert.Equal(t, memory, want)
		assert.NoError(t, err)
	})

	t.Run("fails on unmapped address", func(t *testing.T) {
		mu := GetGenericMu(t)
		want := GenerateCyclic(0x100)
		err := mu.MemWrite(GENERIC_MAPPING_ADDR, want)

		assert.Equal(t, err, UcError(ERR_WRITE_UNMAPPED))
	})
}

func TestRegRead(t *testing.T) {
	t.Run("returns written value for valid register", func(t *testing.T) {
		mu := GetGenericMu(t)
		GetGenericRegisters(t, mu)

		register, err := mu.RegRead(X86_REG_RAX)

		assert.NoError(t, err)
		assert.Equal(t, register, uint64(GENERIC_REGISTER_VALUE))
	})

	t.Run("returns zero for unwritten register", func(t *testing.T) {
		mu := GetGenericMu(t)
		GetGenericRegisters(t, mu)

		register, err := mu.RegRead(X86_REG_RCX)

		assert.NoError(t, err)
		assert.Equal(t, register, uint64(0))
	})

	t.Run("returns zero without error for invalid register", func(t *testing.T) {
		mu := GetGenericMu(t)
		GetGenericRegisters(t, mu)

		register, err := mu.RegRead(INVALID_PLACEHOLDER)

		assert.NoError(t, err) // No error should be returned until unicorn 2.2.0
		assert.Equal(t, register, uint64(0))
	})
}

func TestRegWrite(t *testing.T) {
	t.Run("updates register value", func(t *testing.T) {
		mu := GetGenericMu(t)
		GetGenericRegisters(t, mu)

		register, err := mu.RegRead(X86_REG_RAX)
		assert.NoError(t, err)
		assert.Equal(t, register, uint64(GENERIC_REGISTER_VALUE))

		err = mu.RegWrite(X86_REG_RAX, GENERIC_PLACEHOLDER)
		assert.NoError(t, err)

		register, err = mu.RegRead(X86_REG_RAX)
		assert.NoError(t, err)
		assert.Equal(t, register, uint64(GENERIC_PLACEHOLDER))
	})

	t.Run("does not error on invalid register", func(t *testing.T) {
		mu := GetGenericMu(t)

		err := mu.RegWrite(INVALID_PLACEHOLDER, GENERIC_PLACEHOLDER)
		assert.NoError(t, err) // No error should be returned until unicorn 2.2.0
	})
}

func TestRegReadBatch(t *testing.T) {
	t.Run("returns written values for valid registers", func(t *testing.T) {
		mu := GetGenericMu(t)
		GetGenericRegisters(t, mu)

		registers := []int{X86_REG_RAX, X86_REG_RBX}

		register, err := mu.RegReadBatch(registers)

		assert.NoError(t, err)
		assert.Equal(t, len(registers), 2)
		assert.Equal(t, register[0], uint64(GENERIC_REGISTER_VALUE))
		assert.Equal(t, register[1], uint64(GENERIC_REGISTER_VALUE))
	})

	t.Run("returns zeros for unwritten registers", func(t *testing.T) {
		mu := GetGenericMu(t)

		registers := []int{X86_REG_RAX, X86_REG_RBX}

		register, err := mu.RegReadBatch(registers)

		assert.NoError(t, err)
		assert.Equal(t, len(registers), 2)
		assert.Equal(t, register[0], uint64(0))
		assert.Equal(t, register[1], uint64(0))
	})

	t.Run("returns zeros without error for invalid registers", func(t *testing.T) {
		mu := GetGenericMu(t)

		registers := []int{INVALID_PLACEHOLDER, INVALID_PLACEHOLDER + 1}

		register, err := mu.RegReadBatch(registers)

		assert.NoError(t, err) // No error should be returned until unicorn 2.2.0
		assert.Equal(t, len(registers), 2)
		assert.Equal(t, register[0], uint64(0))
		assert.Equal(t, register[1], uint64(0))
	})

	t.Run("returns value for valid and zero for invalid register", func(t *testing.T) {
		mu := GetGenericMu(t)
		GetGenericRegisters(t, mu)

		registers := []int{X86_REG_RAX, INVALID_PLACEHOLDER}

		register, err := mu.RegReadBatch(registers)

		assert.NoError(t, err) // No error should be returned until unicorn 2.2.0
		assert.Equal(t, len(registers), 2)
		assert.Equal(t, register[0], uint64(GENERIC_REGISTER_VALUE))
		assert.Equal(t, register[1], uint64(0))
	})
}

func TestRegWriteBatch(t *testing.T) {
	t.Run("updates all register values", func(t *testing.T) {
		mu := GetGenericMu(t)
		GetGenericRegisters(t, mu)
		r := []int{X86_REG_RAX, X86_REG_RBX}

		registers, err := mu.RegReadBatch(r)
		assert.NoError(t, err)
		assert.Equal(t, len(registers), 2)
		assert.Equal(t, registers[0], uint64(GENERIC_REGISTER_VALUE))
		assert.Equal(t, registers[1], uint64(GENERIC_REGISTER_VALUE))

		newValues := []uint64{uint64(GENERIC_PLACEHOLDER), uint64(GENERIC_PLACEHOLDER + 1)}
		err = mu.RegWriteBatch(r, newValues)
		assert.NoError(t, err)

		registers, err = mu.RegReadBatch(r)
		assert.NoError(t, err)
		assert.Equal(t, len(registers), 2)
		assert.Equal(t, registers[0], uint64(GENERIC_PLACEHOLDER))
		assert.Equal(t, registers[1], uint64(GENERIC_PLACEHOLDER+1))
	})

	t.Run("does not error on invalid registers", func(t *testing.T) {
		mu := GetGenericMu(t)

		registers := []int{INVALID_PLACEHOLDER, INVALID_PLACEHOLDER + 1}
		values := []uint64{uint64(GENERIC_PLACEHOLDER), uint64(GENERIC_PLACEHOLDER + 1)}

		err := mu.RegWriteBatch(registers, values)
		assert.NoError(t, err) // No error should be returned until unicorn 2.2.0
	})

	t.Run("updates valid register and ignores invalid", func(t *testing.T) {
		mu := GetGenericMu(t)
		GetGenericRegisters(t, mu)
		r := []int{X86_REG_RAX, INVALID_PLACEHOLDER}
		newValues := []uint64{uint64(GENERIC_PLACEHOLDER), uint64(GENERIC_PLACEHOLDER + 1)}

		err := mu.RegWriteBatch(r, newValues)
		assert.NoError(t, err)

		registers, err := mu.RegReadBatch(r)
		assert.NoError(t, err)
		assert.Equal(t, len(registers), 2)
		assert.Equal(t, registers[0], uint64(GENERIC_PLACEHOLDER))
		assert.Equal(t, registers[1], uint64(0))
	})
}

func TestStart(t *testing.T) {
	t.Run("succeeds on mapped executable code", func(t *testing.T) {
		mu := GetGenericMu(t)
		GetGenericCode(t, mu, X86_CODE)

		err := mu.Start(GENERIC_CODE_ADDR, GENERIC_CODE_ADDR+uint64(len(X86_CODE)))
		assert.NoError(t, err)
	})

	t.Run("fails on unmapped address", func(t *testing.T) {
		mu := GetGenericMu(t)

		err := mu.Start(INVALID_PLACEHOLDER, 0)
		assert.Error(t, err, UcError(ERR_FETCH_UNMAPPED))
	})
}

func TestStartWithOptions(t *testing.T) {
	t.Run("stops after count instructions", func(t *testing.T) {
		mu := GetGenericMu(t)
		GetGenericCode(t, mu, X86_CODE)

		err := mu.StartWithOptions(
			GENERIC_CODE_ADDR,
			GENERIC_CODE_ADDR+uint64(len(X86_CODE)),
			&UcOptions{
				Count: 2,
			},
		)
		assert.NoError(t, err)

		ip, err := mu.RegRead(X86_REG_RIP)
		assert.NoError(t, err)
		assert.Equal(t, ip, uint64(GENERIC_CODE_ADDR+2))
	})

	t.Run("correctly emulates INC RAX", func(t *testing.T) {
		mu := GetGenericMu(t)
		GetGenericCode(t, mu, X86_INC_RAX)

		err := mu.RegWrite(X86_REG_RAX, 0x41)
		require.NoError(t, err)

		err = mu.StartWithOptions(
			GENERIC_CODE_ADDR,
			GENERIC_CODE_ADDR+uint64(len(X86_INC_RAX)),
			&UcOptions{},
		)
		assert.NoError(t, err)

		val, err := mu.RegRead(X86_REG_RAX)
		assert.NoError(t, err)
		assert.Equal(t, uint64(0x42), val)
	})

	t.Run("stops infinite loop after timeout", func(t *testing.T) {
		mu := GetGenericMu(t)
		GetGenericCode(t, mu, X86_INFINITE_LOOP)

		done := make(chan struct{})

		go func() {
			select {
			case <-done:
			case <-time.After(20 * time.Second):
				t.Errorf("StartWithOptions did not stop after timeout")
			}
		}()

		err := mu.StartWithOptions(GENERIC_CODE_ADDR, 0, &UcOptions{Timeout: 2_000_000})
		close(done)

		assert.NoError(t, err)
	})
}

func TestStop(t *testing.T) {
	t.Run("halts emulation when called from hook", func(t *testing.T) {
		mu := GetGenericMu(t)
		GetGenericCode(t, mu, X86_CODE)

		_, err := mu.HookAdd(HOOK_CODE, func(muHook Unicorn, addr uint64, size uint32) {
			muHook.Stop()
		}, GENERIC_CODE_ADDR, GENERIC_CODE_ADDR+uint64(len(X86_CODE)))
		require.NoError(t, err)

		err = mu.Start(GENERIC_CODE_ADDR, GENERIC_CODE_ADDR+uint64(len(X86_CODE)))
		assert.NoError(t, err)

		ip, err := mu.RegRead(X86_REG_RIP)
		assert.NoError(t, err)
		assert.Equal(t, ip, uint64(GENERIC_CODE_ADDR)) // ip should not be incremented
	})
}

func TestHookAdd(t *testing.T) {
	t.Run("HOOK_CODE fires for each instruction", func(t *testing.T) {
		mu := GetGenericMu(t)
		GetGenericCode(t, mu, X86_CODE)

		called := 0
		_, err := mu.HookAdd(HOOK_CODE, func(muHook Unicorn, addr uint64, size uint32) {
			called++
		}, GENERIC_CODE_ADDR, GENERIC_CODE_ADDR+uint64(len(X86_CODE)))
		require.NoError(t, err)

		err = mu.Start(GENERIC_CODE_ADDR, GENERIC_CODE_ADDR+uint64(len(X86_CODE)))
		assert.NoError(t, err)
		assert.Equal(t, 4, called) // NOP;NOP;NOP;HLT should equal 4 instructions
	})

	t.Run("HOOK_MEM_WRITE does not fire on direct MemWrite", func(t *testing.T) {
		mu := GetGenericMu(t)
		GetGenericCode(t, mu, X86_CODE)

		written := false
		_, err := mu.HookAdd(
			HOOK_MEM_WRITE,
			func(muHook Unicorn, access int, addr uint64, size int, value int64) {
				written = true
			},
			1,
			0,
		)
		require.NoError(t, err)

		err = mu.MemWrite(GENERIC_MAPPING_ADDR+0x100, GenerateCyclic(4))
		require.NoError(t, err)

		assert.False(t, written)
	})
}

func TestHookDel(t *testing.T) {
	t.Run("hook no longer fires after deletion", func(t *testing.T) {
		mu := GetGenericMu(t)
		GetGenericCode(t, mu, X86_CODE)

		called := 0
		hook, err := mu.HookAdd(HOOK_CODE, func(muHook Unicorn, addr uint64, size uint32) {
			called++
		}, GENERIC_CODE_ADDR, GENERIC_CODE_ADDR+uint64(len(X86_CODE)))
		require.NoError(t, err)

		err = mu.HookDel(hook)
		require.NoError(t, err)

		err = mu.Start(GENERIC_CODE_ADDR, GENERIC_CODE_ADDR+uint64(len(X86_CODE)))
		assert.NoError(t, err)
		assert.Equal(t, 0, called)
	})
}

func TestContextSave(t *testing.T) {
	t.Run("returns valid non-nil context", func(t *testing.T) {
		mu := GetGenericMu(t)

		ctx, err := mu.ContextSave(nil)
		assert.NoError(t, err)
		assert.NotNil(t, ctx)
	})
}

func TestContextRestore(t *testing.T) {
	t.Run("restores register state from saved context", func(t *testing.T) {
		mu := GetGenericMu(t)

		err := mu.RegWrite(X86_REG_RAX, GENERIC_PLACEHOLDER)
		require.NoError(t, err)

		ctx, err := mu.ContextSave(nil)
		require.NoError(t, err)

		err = mu.RegWrite(X86_REG_RAX, GENERIC_PLACEHOLDER+1)
		require.NoError(t, err)

		err = mu.ContextRestore(ctx)
		assert.NoError(t, err)

		val, err := mu.RegRead(X86_REG_RAX)
		assert.NoError(t, err)
		assert.Equal(t, uint64(GENERIC_PLACEHOLDER), val)
	})
}

func TestQuery(t *testing.T) {
	t.Run("QUERY_MODE returns correct emulation mode", func(t *testing.T) {
		mu := GetGenericMu(t)

		mode, err := mu.Query(QUERY_MODE)
		assert.NoError(t, err)
		assert.Equal(t, uint64(MODE_64), mode)
	})

	t.Run("QUERY_PAGE_SIZE returns page size", func(t *testing.T) {
		mu := GetGenericMu(t)

		pageSize, err := mu.Query(QUERY_PAGE_SIZE)
		assert.NoError(t, err)
		assert.Equal(t, pageSize, uint64(GENERIC_MAPPING_SIZE))
	})

	t.Run("fails with invalid query type", func(t *testing.T) {
		mu := GetGenericMu(t)

		pageSize, err := mu.Query(INVALID_PLACEHOLDER)
		assert.Equal(t, err, UcError(ERR_ARG))
		assert.Equal(t, pageSize, uint64(0))
	})
}

func TestGetMode(t *testing.T) {
	t.Run("returns MODE_64 for X86_64 instance", func(t *testing.T) {
		mu := GetGenericMu(t)

		mode, err := mu.GetMode()
		assert.NoError(t, err)
		assert.Equal(t, MODE_64, mode)
	})
}

func TestGetArch(t *testing.T) {
	t.Run("returns ARCH_X86 for X86_64 instance", func(t *testing.T) {
		mu := GetGenericMu(t)

		arch, err := mu.GetArch()
		assert.NoError(t, err)
		assert.Equal(t, ARCH_X86, arch)
	})
}

func TestGetPageSize(t *testing.T) {
	t.Run("returns default page size", func(t *testing.T) {
		mu := GetGenericMu(t)

		size, err := mu.GetPageSize()
		assert.NoError(t, err)
		assert.Equal(t, size, uint32(GENERIC_MAPPING_SIZE))
	})
}

func TestSetPageSize(t *testing.T) {
	t.Run("persists new page size", func(t *testing.T) {
		mu, err := NewUnicorn(ARCH_ARM64, MODE_ARM)
		require.NoError(t, err)

		err = mu.SetPageSize(GENERIC_MAPPING_SIZE * 2)
		assert.NoError(t, err)

		current, err := mu.GetPageSize()
		assert.NoError(t, err)
		assert.Equal(t, current, uint32(GENERIC_MAPPING_SIZE*2))
	})
}

func TestGetTimeout(t *testing.T) {
	t.Run("returns zero before emulation", func(t *testing.T) {
		mu := GetGenericMu(t)

		timeout, err := mu.GetTimeout()
		assert.NoError(t, err)
		assert.Equal(t, uint64(0), timeout)
	})
}

func TestGetCPUModel(t *testing.T) {
	t.Run("returns default CPU model", func(t *testing.T) {
		mu := GetGenericMu(t)

		model, err := mu.GetCPUModel()
		assert.NoError(t, err)
		assert.Equal(t, model, CPU_X86_HASWELL)
	})
}

func TestSetCPUModel(t *testing.T) {
	t.Run("persists new CPU model", func(t *testing.T) {
		mu := GetGenericMu(t)

		err := mu.SetCPUModel(CPU_X86_CORE2DUO)
		assert.NoError(t, err)

		model, err := mu.GetCPUModel()
		assert.NoError(t, err)
		assert.Equal(t, model, CPU_X86_CORE2DUO)
	})
}

func TestExits(t *testing.T) {
	t.Run("ExitsEnable succeeds", func(t *testing.T) {
		mu := GetGenericMu(t)
		assert.NoError(t, mu.ExitsEnable())
	})

	t.Run("ExitsDisable succeeds after enable", func(t *testing.T) {
		mu := GetGenericMu(t)
		require.NoError(t, mu.ExitsEnable())
		assert.NoError(t, mu.ExitsDisable())
	})

	t.Run("SetExits and GetExits are consistent", func(t *testing.T) {
		mu := GetGenericMu(t)
		require.NoError(t, mu.ExitsEnable())

		exits := []uint64{0x1000, 0x2000, 0x3000}
		err := mu.SetExits(exits)
		require.NoError(t, err)

		got, err := mu.GetExits()
		assert.NoError(t, err)
		assert.ElementsMatch(t, exits, got)
	})
}

func TestTCGBufferSize(t *testing.T) {
	t.Run("persists new buffer size", func(t *testing.T) {
		mu := GetGenericMu(t)

		original, err := mu.GetTCGBufferSize()
		require.NoError(t, err)

		err = mu.SetTCGBufferSize(original * 2)
		assert.NoError(t, err)

		current, err := mu.GetTCGBufferSize()
		assert.NoError(t, err)
		assert.Equal(t, current, original*2)
	})
}

func TestFlushTB(t *testing.T) {
	t.Run("succeeds on idle engine", func(t *testing.T) {
		mu := GetGenericMu(t)
		assert.NoError(t, mu.FlushTB())
	})

	// TODO: Add a FlushTB test on real emulation
	// by doing dynamic code override
}

func TestFlushTLB(t *testing.T) {
	t.Run("succeeds on idle engine", func(t *testing.T) {
		mu := GetGenericMu(t)
		assert.NoError(t, mu.FlushTLB())
	})
}

func TestTLBMode(t *testing.T) {
	t.Run("TLB_VIRTUAL mode is accepted", func(t *testing.T) {
		mu := GetGenericMu(t)
		assert.NoError(t, mu.TLBMode(TLB_VIRTUAL))
	})
}

func TestRequestCache(t *testing.T) {
	t.Run("returns translation block after execution", func(t *testing.T) {
		mu := GetGenericMu(t)
		GetGenericCode(t, mu, X86_CODE)

		err := mu.Start(GENERIC_CODE_ADDR, GENERIC_CODE_ADDR+uint64(len(X86_CODE)))
		require.NoError(t, err)

		tb, err := mu.RequestCache(GENERIC_CODE_ADDR)
		assert.NoError(t, err)
		assert.Equal(t, uint64(GENERIC_CODE_ADDR), tb.Pc)
		assert.Equal(t, uint16(len(X86_CODE)), tb.Size)
	})
}

func TestRemoveCache(t *testing.T) {
	t.Run("succeeds on previously executed region", func(t *testing.T) {
		mu := GetGenericMu(t)
		GetGenericCode(t, mu, X86_CODE)

		err := mu.Start(GENERIC_CODE_ADDR, GENERIC_CODE_ADDR+uint64(len(X86_CODE)))
		require.NoError(t, err)

		err = mu.RemoveCache(GENERIC_CODE_ADDR, GENERIC_CODE_ADDR+uint64(GENERIC_CODE_SIZE))
		assert.NoError(t, err)
	})
}

func TestContextMode(t *testing.T) {
	t.Run("CTL_CONTEXT_CPU is accepted", func(t *testing.T) {
		mu := GetGenericMu(t)
		assert.NoError(t, mu.ContextMode(CTL_CONTEXT_CPU))
	})
}
