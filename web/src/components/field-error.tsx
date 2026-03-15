export function FieldError({ message }: { message?: string }) {
  if (!message) {
    return null;
  }

  return (
    <p role="alert" aria-live="polite" className="text-destructive text-xs">
      {message}
    </p>
  );
}
