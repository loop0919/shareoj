import { expect, test } from '@playwright/test'
import { epsilonExponent, errorJudgeModeLabel, thresholdFactor, validMargin } from '../app/utils/error-judge'

test('しきい値の倍率を正確に求める', () => {
  expect(thresholdFactor('0.1')).toBe('1.001')
  expect(thresholdFactor('0')).toBe('1')
  expect(thresholdFactor('100')).toBe('2')
  expect(thresholdFactor('12.345678')).toBe('1.12345678')
  expect(thresholdFactor('0.000001')).toBe('1.00000001')
  expect(thresholdFactor('50')).toBe('1.5')
  // Trailing zeros of the margin do not show in the factor, and 100 written with decimals is still 2.
  expect(thresholdFactor('0.0')).toBe('1')
  expect(thresholdFactor('99.999999')).toBe('1.99999999')
  expect(thresholdFactor('100.000000')).toBe('2')
  expect(thresholdFactor('099')).toBe('1.99')
  expect(thresholdFactor('000')).toBe('1')
  expect(() => thresholdFactor('101')).toThrow()
})

test('余裕の形を検査する', () => {
  for (const text of ['0', '0.5', '100', '12.345678', '100.0', '007', '099', '000']) expect(validMargin(text), text).toBe(true)
  for (const text of ['', '101', '100.000001', '-1', '1e1', '.5', '0.1.2', '0.1234567', '1.', ' 0.5', '1000']) expect(validMargin(text), text).toBe(false)
})

test('許容誤差の指数と種類の名前', () => {
  expect(epsilonExponent('1e-6')).toBe(-6)
  expect(epsilonExponent('1e-1')).toBe(-1)
  expect(epsilonExponent('1e-12')).toBe(-12)
  expect(errorJudgeModeLabel('absolute')).toBe('絶対誤差')
  expect(errorJudgeModeLabel('relative')).toBe('相対誤差')
  expect(errorJudgeModeLabel('either')).toBe('絶対誤差または相対誤差')
})
