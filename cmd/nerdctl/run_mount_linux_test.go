/*
   Copyright The containerd Authors.

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

package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
	"gotest.tools/v3/assert"
)

func TestRunVolume(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	rwDir, err := os.MkdirTemp("", "nerdctl-"+t.Name()+"-rw")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(rwDir)
	roDir, err := os.MkdirTemp("", "nerdctl-"+t.Name()+"-ro")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(roDir)
	rwVolName := "nerdctl-testrunvolume-rw"
	roVolName := "nerdctl-testrunvolume-ro"
	for _, v := range []string{rwVolName, roVolName} {
		defer base.Cmd("volume", "rm", "-f", v).Run()
		base.Cmd("volume", "create", v).AssertOK()
	}

	containerName := "nerdctl-testrunvolume"
	defer base.Cmd("rm", "-f", containerName).Run()
	base.Cmd("run",
		"-d",
		"--name", containerName,
		"-v", fmt.Sprintf("%s:/mnt1", rwDir),
		"-v", fmt.Sprintf("%s:/mnt2:ro", roDir),
		"-v", fmt.Sprintf("%s:/mnt3", rwVolName),
		"-v", fmt.Sprintf("%s:/mnt4:ro", roVolName),
		testutil.AlpineImage,
		"top",
	).AssertOK()
	base.Cmd("exec", containerName, "sh", "-exc", "echo -n str1 > /mnt1/file1").AssertOK()
	base.Cmd("exec", containerName, "sh", "-exc", "echo -n str2 > /mnt2/file2").AssertFail()
	base.Cmd("exec", containerName, "sh", "-exc", "echo -n str3 > /mnt3/file3").AssertOK()
	base.Cmd("exec", containerName, "sh", "-exc", "echo -n str4 > /mnt4/file4").AssertFail()
	base.Cmd("rm", "-f", containerName).AssertOK()
	base.Cmd("run",
		"--rm",
		"-v", fmt.Sprintf("%s:/mnt1", rwDir),
		"-v", fmt.Sprintf("%s:/mnt3", rwVolName),
		testutil.AlpineImage,
		"cat", "/mnt1/file1", "/mnt3/file3",
	).AssertOutExactly("str1str3")
}

func TestRunAnonymousVolume(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	base.Cmd("run", "--rm", "-v", "/foo", testutil.AlpineImage,
		"mountpoint", "-q", "/foo").AssertOK()
}

func TestRunAnonymousVolumeWithBuild(t *testing.T) {
	t.Parallel()
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	const imageName = "nerdctl-test-anonymous-volume-with-build"
	defer base.Cmd("rmi", imageName).Run()

	dockerfile := fmt.Sprintf(`FROM %s
VOLUME /foo
        `, testutil.AlpineImage)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	base.Cmd("build", "-t", imageName, buildCtx).AssertOK()
	base.Cmd("run", "--rm", "-v", "/foo", testutil.AlpineImage,
		"mountpoint", "-q", "/foo").AssertOK()
}

func TestRunCopyingUpInitialContentsOnVolume(t *testing.T) {
	t.Parallel()
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	const imageName = "nerdctl-test-copying-on-volume"
	defer base.Cmd("rmi", imageName).Run()
	defer base.Cmd("volume", "rm", "copying-initial-content-on-volume").Run()

	dockerfile := fmt.Sprintf(`FROM %s
RUN mkdir -p /mnt && echo hi > /mnt/initial_file
CMD ["cat", "/mnt/initial_file"]
        `, testutil.AlpineImage)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	base.Cmd("build", "-t", imageName, buildCtx).AssertOK()

	//AnonymousVolume
	base.Cmd("run", "--rm", imageName).AssertOutExactly("hi\n")
	base.Cmd("run", "-v", "/mnt", "--rm", imageName).AssertOutExactly("hi\n")

	//NamedVolume should be automatically created
	base.Cmd("run", "-v", "copying-initial-content-on-volume:/mnt", "--rm", imageName).AssertOutExactly("hi\n")
}

func TestRunCopyingUpInitialContentsOnDockerfileVolume(t *testing.T) {
	t.Parallel()
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	const imageName = "nerdctl-test-copying-initial-content"
	defer base.Cmd("rmi", imageName).Run()
	defer base.Cmd("volume", "rm", "copying-initial-content").Run()

	dockerfile := fmt.Sprintf(`FROM %s
RUN mkdir -p /mnt && echo hi > /mnt/initial_file
VOLUME /mnt
CMD ["cat", "/mnt/initial_file"]
        `, testutil.AlpineImage)

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	base.Cmd("build", "-t", imageName, buildCtx).AssertOK()
	//AnonymousVolume
	base.Cmd("run", "--rm", imageName).AssertOutExactly("hi\n")
	base.Cmd("run", "-v", "/mnt", "--rm", imageName).AssertOutExactly("hi\n")

	//NamedVolume
	base.Cmd("volume", "create", "copying-initial-content").AssertOK()
	base.Cmd("run", "-v", "copying-initial-content:/mnt", "--rm", imageName).AssertOutExactly("hi\n")

	//mount bind
	tmpDir, err := os.MkdirTemp("", "hostDir")
	assert.NilError(t, err)
	defer os.RemoveAll(tmpDir)

	base.Cmd("run", "-v", fmt.Sprintf("%s:/mnt", tmpDir), "--rm", imageName).AssertFail()
}

func TestRunCopyingUpInitialContentsOnVolumeShouldRetainSymlink(t *testing.T) {
	t.Parallel()
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	const imageName = "nerdctl-test-copy-up-retain-symlink"
	defer base.Cmd("rmi", imageName).Run()

	dockerfile := fmt.Sprintf(`FROM %s
RUN ln -s ../../../../../../../../../../../../../../../../../../etc/passwd /mnt/passwd
VOLUME /mnt
CMD ["readlink", "/mnt/passwd"]
        `, testutil.AlpineImage)
	const expected = "../../../../../../../../../../../../../../../../../../etc/passwd\n"

	buildCtx, err := createBuildContext(dockerfile)
	assert.NilError(t, err)
	defer os.RemoveAll(buildCtx)

	base.Cmd("build", "-t", imageName, buildCtx).AssertOK()

	base.Cmd("run", "--rm", imageName).AssertOutExactly(expected)
	base.Cmd("run", "-v", "/mnt", "--rm", imageName).AssertOutExactly(expected)
}

func TestRunTmpfs(t *testing.T) {
	t.Parallel()
	base := testutil.NewBase(t)
	f := func(allow, deny []string) func(stdout string) error {
		return func(stdout string) error {
			for _, s := range allow {
				if !strings.Contains(stdout, s) {
					return fmt.Errorf("expected stdout to contain %q, got %q", s, stdout)
				}
			}
			for _, s := range deny {
				if strings.Contains(stdout, s) {
					return fmt.Errorf("expected stdout not to contain %q, got %q", s, stdout)
				}
			}
			return nil
		}
	}
	base.Cmd("run", "--rm", "--tmpfs", "/tmp", testutil.AlpineImage, "grep", "/tmp", "/proc/mounts").AssertOutWithFunc(f([]string{"rw", "nosuid", "nodev", "noexec"}, nil))
	base.Cmd("run", "--rm", "--tmpfs", "/tmp:size=64m,exec", testutil.AlpineImage, "grep", "/tmp", "/proc/mounts").AssertOutWithFunc(f([]string{"rw", "nosuid", "nodev", "size=65536k"}, []string{"noexec"}))
}