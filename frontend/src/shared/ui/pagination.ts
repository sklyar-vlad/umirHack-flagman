export function paginationItems(page: number, pageCount: number): (number | 'start-gap' | 'end-gap')[] {
  if (!Number.isSafeInteger(pageCount) || pageCount < 1 || !Number.isSafeInteger(page) || page < 1 || page > pageCount) throw new RangeError('Invalid pagination range')
  const pages = [...new Set([1, pageCount, page - 1, page, page + 1].filter((value) => value >= 1 && value <= pageCount))].sort((a, b) => a - b)
  const result: (number | 'start-gap' | 'end-gap')[] = []
  pages.forEach((value, index) => {
    const previous = pages[index - 1]
    if (previous && value - previous > 1) result.push(value < page ? 'start-gap' : 'end-gap')
    result.push(value)
  })
  return result
}
