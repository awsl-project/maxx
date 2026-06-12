export default async function globalTeardown() {
  const mockPid = process.env.MOCK_PID;
  const maxxPid = process.env.MAXX_PID;

  if (maxxPid) {
    try {
      process.kill(Number(maxxPid));
    } catch { /* process already exited */ }
  }
  if (mockPid) {
    try {
      process.kill(Number(mockPid));
    } catch { /* process already exited */ }
  }
}
