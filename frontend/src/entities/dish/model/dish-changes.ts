import { createAction, createReducer } from '@reduxjs/toolkit'
/** An uncertain write also makes any previous whole-basket check unsafe. */
export const dishReadsChanged = createAction('dishes/readsChanged')
export const dishChangesReducer = createReducer(0, (builder) => builder.addCase(dishReadsChanged, (state) => state + 1))
export const selectDishChanges = (state: { dishChanges: number }) => state.dishChanges
