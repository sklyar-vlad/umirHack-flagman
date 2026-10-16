import { createAction, createReducer } from '@reduxjs/toolkit'
export const dishReadsChanged = createAction('dishes/readsChanged')
export const dishChangesReducer = createReducer(0, (builder) => builder.addCase(dishReadsChanged, (state) => state + 1))
export const selectDishChanges = (state: { dishChanges: number }) => state.dishChanges
