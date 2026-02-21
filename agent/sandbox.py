import docker
import os
import tarfile
import io

class DockerSandbox:
    def __init__(self):
        try:
            self.client = docker.from_env()
        except docker.errors.DockerException as e:
            raise RuntimeError(
                "Docker daemon not found or not running. Start Docker Desktop (or ensure your Docker daemon is accessible via DOCKER_HOST), then retry. Original error: "
                + str(e)
            )
        # Use a Raven-specific tag for the local sandbox image
        self.image_tag = "raven-sandbox:latest"
        self.timeout_seconds = _int_env("DOCKER_TIMEOUT", 30)

    def build_image(self):
        """Ensures the sandbox image exists."""
        path = os.path.join(os.getcwd(), 'sandbox_env')
        print(f"🐳 Building Sandbox Image from {path}...")
        self.client.images.build(path=path, tag=self.image_tag)

    def run_verification(self, code_patch, test_script_sh):
        """
        Spins up a container, injects a bash script, and runs it to pull repo and test.
        Returns: {success: bool, logs: str}
        """
        container = None
        try:
            # We now allow network access to clone real GitHub repos.
            container = self.client.containers.run(
                self.image_tag,
                command="/bin/bash /app/run_tests.sh",
                detach=True,
                mem_limit="512m",  # Expanded for pip installs
                pids_limit=128,
            )

            # Inject the patch and the execution shell script
            self._copy_to_container(container, "solution.py", code_patch)
            self._copy_to_container(container, "run_tests.sh", test_script_sh)

            # Wait for execution with timeout
            try:
                result = container.wait(timeout=self.timeout_seconds)
            except Exception:
                try:
                    container.kill()
                except Exception:
                    pass
                return {"success": False, "logs": f"Sandbox timeout after {self.timeout_seconds}s"}

            logs = container.logs().decode("utf-8", errors="replace")
            success = result.get("StatusCode", 1) == 0
            return {"success": success, "logs": logs}
        except Exception as e:
            return {"success": False, "logs": str(e)}
        finally:
            if container is not None:
                try:
                    container.remove(force=True)
                except Exception:
                    pass

    def _copy_to_container(self, container, filename, content):
        """Helper to inject in-memory files into Docker"""
        tar_stream = io.BytesIO()
        with tarfile.open(fileobj=tar_stream, mode='w') as tar:
            data = content.encode('utf-8')
            tarinfo = tarfile.TarInfo(name=filename)
            tarinfo.size = len(data)
            tar.addfile(tarinfo, io.BytesIO(data))
        tar_stream.seek(0)
        container.put_archive('/app', tar_stream)


def _int_env(name: str, default: int) -> int:
    try:
        v = os.getenv(name)
        return int(v) if v is not None else default
    except ValueError:
        return default
