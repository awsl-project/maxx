export interface CopyUserPanelTokenOptions {
  revealedToken?: string;
  revealToken: () => Promise<string>;
  writeText: (value: string) => Promise<void>;
}

export async function copyUserPanelToken({
  revealedToken,
  revealToken,
  writeText,
}: CopyUserPanelTokenOptions): Promise<string> {
  const token = revealedToken || (await revealToken());
  await writeText(token);
  return token;
}
