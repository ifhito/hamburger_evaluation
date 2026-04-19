export function formatRating(rating: number): string {
  return "★".repeat(rating) + "☆".repeat(5 - rating);
}

export function formatConfidence(confidence: number): string {
  return `${(confidence * 100).toFixed(0)}%`;
}
