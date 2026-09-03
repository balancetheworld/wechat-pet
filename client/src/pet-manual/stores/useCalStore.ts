/**
 * 宠物小册共享数据（跨 tab 页）
 *
 * 来源：原 Pet-Manual 单页（pet-manual/pages/index/index.tsx）把宠物日历的记录/待办
 * 提升在页面内共享（档案册的「成长足迹」添加 → 日历板块可见）。拆成 tab 独立页面后，
 * 记录/待办改为本全局 store 承载，档案页写入、日历页读取/修改。
 */
import { create } from 'zustand'
import type { Dispatch, SetStateAction } from 'react'
import type { CalRecord, CalTodo } from '../pages/index/data'
import { INIT_CAL_RECORDS, INIT_CAL_TODOS } from '../pages/index/data'

/** 兼容 Dispatch<SetStateAction<T>>：既支持直接赋值，也支持函数式更新 */
const resolve = <T,>(value: SetStateAction<T>, prev: T): T =>
  typeof value === 'function' ? (value as (p: T) => T)(prev) : value

interface CalStoreState {
  records: CalRecord[]
  todos: CalTodo[]
  setRecords: Dispatch<SetStateAction<CalRecord[]>>
  setTodos: Dispatch<SetStateAction<CalTodo[]>>
  addRecord: (record: CalRecord) => void
  addTodo: (todo: CalTodo) => void
}

export const useCalStore = create<CalStoreState>(set => ({
  records: INIT_CAL_RECORDS,
  todos: INIT_CAL_TODOS,
  setRecords: value => set(s => ({ records: resolve(value, s.records) })),
  setTodos: value => set(s => ({ todos: resolve(value, s.todos) })),
  addRecord: record => set(s => ({ records: [...s.records, record] })),
  addTodo: todo => set(s => ({ todos: [...s.todos, todo] })),
}))
