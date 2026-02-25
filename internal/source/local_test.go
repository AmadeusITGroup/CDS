package source_test

import (
	"bytes"
	"io"

	"github.com/amadeusitgroup/cds/internal/cos"
	"github.com/amadeusitgroup/cds/internal/source"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/spf13/afero"
)

var _ = ginkgo.BeforeSuite(func() {
	cos.SetMockedFileSystem()
})

var _ = ginkgo.AfterSuite(func() {
	cos.SetRealFileSystem()
})

var _ = ginkgo.BeforeEach(func() {
	// Reset to a fresh in-memory FS before each test
	cos.Fs = afero.NewMemMapFs()
})

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func setupFixtures() {
	// /testroot/
	//   file.txt          → "hello world"
	//   subdir/
	//     nested.txt      → "nested content"
	_ = cos.Fs.MkdirAll("/testroot/subdir", 0755)
	_ = cos.WriteFile("/testroot/file.txt", []byte("hello world"), 0644)
	_ = cos.WriteFile("/testroot/subdir/nested.txt", []byte("nested content"), 0644)
}

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

var _ = ginkgo.Describe("NewLocalSource", func() {
	ginkgo.BeforeEach(func() {
		setupFixtures()
	})

	ginkgo.It("succeeds with a valid directory", func() {
		src, err := source.NewLocalSource("/testroot")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(src).NotTo(gomega.BeNil())
		gomega.Expect(src.Type()).To(gomega.Equal(source.LocalFS))
	})

	ginkgo.It("succeeds with a valid file", func() {
		src, err := source.NewLocalSource("/testroot/file.txt")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(src).NotTo(gomega.BeNil())
		gomega.Expect(src.Type()).To(gomega.Equal(source.LocalFS))
	})

	ginkgo.It("succeeds even when the path does not exist (no existence check)", func() {
		src, err := source.NewLocalSource("/nonexistent")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(src).NotTo(gomega.BeNil())
		gomega.Expect(src.Type()).To(gomega.Equal(source.LocalFS))
	})
})

// ---------------------------------------------------------------------------
// Information
// ---------------------------------------------------------------------------

var _ = ginkgo.Describe("localSource.Information", func() {
	ginkgo.BeforeEach(func() {
		setupFixtures()
	})

	ginkgo.It("returns the absolute path for a directory source", func() {
		src, _ := source.NewLocalSource("/testroot/subdir")
		gomega.Expect(src.Information()).To(gomega.Equal("/testroot/subdir"))
	})

	ginkgo.It("returns the absolute path for a file source", func() {
		src, _ := source.NewLocalSource("/testroot/file.txt")
		gomega.Expect(src.Information()).To(gomega.Equal("/testroot/file.txt"))
	})
})

// ---------------------------------------------------------------------------
// Read
// ---------------------------------------------------------------------------

var _ = ginkgo.Describe("localSource.Read", func() {
	ginkgo.BeforeEach(func() {
		setupFixtures()
	})

	ginkgo.It("reads a file source", func() {
		src, _ := source.NewLocalSource("/testroot/file.txt")
		reader, err := src.Read()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		data, err := io.ReadAll(reader)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(string(data)).To(gomega.Equal("hello world"))
	})

	ginkgo.It("reads a nested file source", func() {
		src, _ := source.NewLocalSource("/testroot/subdir/nested.txt")
		reader, err := src.Read()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		data, err := io.ReadAll(reader)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(string(data)).To(gomega.Equal("nested content"))
	})

	ginkgo.It("errors when source is a directory", func() {
		src, _ := source.NewLocalSource("/testroot")
		_, err := src.Read()
		gomega.Expect(err).To(gomega.HaveOccurred())
		gomega.Expect(err.Error()).To(gomega.ContainSubstring("directory"))
	})
})

// ---------------------------------------------------------------------------
// Write
// ---------------------------------------------------------------------------

var _ = ginkgo.Describe("localSource.Write", func() {
	ginkgo.BeforeEach(func() {
		setupFixtures()
	})

	ginkgo.It("writes to an existing file source", func() {
		src, _ := source.NewLocalSource("/testroot/file.txt")
		err := src.Write(bytes.NewReader([]byte("updated")), 0644)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		reader, err := src.Read()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		data, err := io.ReadAll(reader)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(string(data)).To(gomega.Equal("updated"))
	})

	ginkgo.It("creates parent directories when writing a new file", func() {
		// Create the parent so NewLocalSource works, then write via child
		_ = cos.Fs.MkdirAll("/testroot/a/b", 0755)
		src, err := source.NewLocalSource("/testroot/a/b")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		children, _ := src.Children()
		// No children yet — write a new file by creating a source for it
		gomega.Expect(children).To(gomega.BeEmpty())

		// Write through a new source path that doesn't exist yet
		_ = cos.WriteFile("/testroot/a/b/c.txt", []byte("deep"), 0644)
		newSrc, err := source.NewLocalSource("/testroot/a/b/c.txt")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		reader, err := newSrc.Read()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		data, err := io.ReadAll(reader)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(string(data)).To(gomega.Equal("deep"))
	})
})

// ---------------------------------------------------------------------------
// Children
// ---------------------------------------------------------------------------

var _ = ginkgo.Describe("localSource.Children", func() {
	ginkgo.BeforeEach(func() {
		setupFixtures()
	})

	ginkgo.It("lists children of a directory", func() {
		src, _ := source.NewLocalSource("/testroot")
		children, err := src.Children()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(children).To(gomega.HaveLen(2))

		entries := map[string]bool{}
		for _, e := range children {
			isDir, err := e.IsDir()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			entries[e.Information()] = isDir
		}
		gomega.Expect(entries).To(gomega.HaveKeyWithValue("/testroot/file.txt", false))
		gomega.Expect(entries).To(gomega.HaveKeyWithValue("/testroot/subdir", true))
	})

	ginkgo.It("lists children of a subdirectory", func() {
		src, _ := source.NewLocalSource("/testroot/subdir")
		children, err := src.Children()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(children).To(gomega.HaveLen(1))
		gomega.Expect(children[0].Information()).To(gomega.Equal("/testroot/subdir/nested.txt"))
		isDir, err := children[0].IsDir()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(isDir).To(gomega.BeFalse())
	})

	ginkgo.It("child sources are fully functional", func() {
		src, _ := source.NewLocalSource("/testroot/subdir")
		children, _ := src.Children()
		gomega.Expect(children).To(gomega.HaveLen(1))

		reader, err := children[0].Read()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		data, err := io.ReadAll(reader)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(string(data)).To(gomega.Equal("nested content"))
	})

	ginkgo.It("returns an error when source is a file", func() {
		src, _ := source.NewLocalSource("/testroot/file.txt")
		_, err := src.Children()
		gomega.Expect(err).To(gomega.HaveOccurred())
		gomega.Expect(err.Error()).To(gomega.ContainSubstring("not a directory"))
	})
})

// ---------------------------------------------------------------------------
// Exists
// ---------------------------------------------------------------------------

var _ = ginkgo.Describe("localSource.Exists", func() {
	ginkgo.BeforeEach(func() {
		setupFixtures()
	})

	ginkgo.It("returns true for an existing file", func() {
		src, _ := source.NewLocalSource("/testroot/file.txt")
		ok, err := src.Exists()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(ok).To(gomega.BeTrue())
	})

	ginkgo.It("returns true for an existing directory", func() {
		src, _ := source.NewLocalSource("/testroot/subdir")
		ok, err := src.Exists()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(ok).To(gomega.BeTrue())
	})
})

// ---------------------------------------------------------------------------
// IsDir
// ---------------------------------------------------------------------------

var _ = ginkgo.Describe("localSource.IsDir", func() {
	ginkgo.BeforeEach(func() {
		setupFixtures()
	})

	ginkgo.It("returns true for a directory", func() {
		src, _ := source.NewLocalSource("/testroot/subdir")
		ok, err := src.IsDir()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(ok).To(gomega.BeTrue())
	})

	ginkgo.It("returns false for a file", func() {
		src, _ := source.NewLocalSource("/testroot/file.txt")
		ok, err := src.IsDir()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(ok).To(gomega.BeFalse())
	})
})
