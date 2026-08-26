// A four-line allocator shim, and the reason it has to exist.
//
// Manifold's C API constructs its objects with placement new into a buffer the
// caller supplies, and destroys them with `delete`. That means the buffer has
// to come from C++'s `operator new`, because `delete` ends in `operator delete`
// and the two must be a matching pair. Handing it `malloc` memory instead is
// undefined behaviour, and on this MinGW toolchain it is not the harmless kind:
// libstdc++'s `operator delete` and the UCRT's `free` do not share a heap, so
// the first destroy walks into memory that was never theirs and the process
// dies inside the deleter.
//
// Manifold's own C tests dodge this by never destroying anything and letting
// the process exit clean up. A modelling session that runs a boolean per click
// cannot leak a mesh each time, so it allocates the right way instead.

#include <cstddef>
#include <new>

extern "C" {

// csgAlloc returns memory that Manifold's `delete` can release, or null.
void* csgAlloc(size_t n) { return ::operator new(n, std::nothrow); }

// csgFree releases memory from csgAlloc that was never handed to Manifold.
void csgFree(void* p) { ::operator delete(p); }
}
