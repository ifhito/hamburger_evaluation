import { useState } from 'react'
import type { LoginRequest, SignupRequest } from '../types/auth'

interface SignupFormState {
  username: string
  email: string
  password: string
  password_confirmation: string
}

interface SignupFormErrors {
  username?: string
  email?: string
  password?: string
  password_confirmation?: string
}

export function useSignupForm() {
  const [fields, setFields] = useState<SignupFormState>({
    username: '',
    email: '',
    password: '',
    password_confirmation: '',
  })
  const [errors, setErrors] = useState<SignupFormErrors>({})

  const setField = <K extends keyof SignupFormState>(key: K, value: string) => {
    setFields((prev) => ({ ...prev, [key]: value }))
    setErrors((prev) => ({ ...prev, [key]: undefined }))
  }

  const validate = (): boolean => {
    const newErrors: SignupFormErrors = {}
    if (!fields.username.trim()) newErrors.username = 'Username is required'
    if (!fields.email.trim()) newErrors.email = 'Email is required'
    if (fields.password.length < 6) newErrors.password = 'Password must be at least 6 characters'
    if (fields.password !== fields.password_confirmation) {
      newErrors.password_confirmation = "Passwords don't match"
    }
    setErrors(newErrors)
    return Object.keys(newErrors).length === 0
  }

  const toRequest = (): SignupRequest => fields

  return { fields, errors, setField, validate, toRequest }
}

interface LoginFormState {
  email: string
  password: string
}

interface LoginFormErrors {
  email?: string
  password?: string
}

export function useLoginForm() {
  const [fields, setFields] = useState<LoginFormState>({ email: '', password: '' })
  const [errors, setErrors] = useState<LoginFormErrors>({})

  const setField = <K extends keyof LoginFormState>(key: K, value: string) => {
    setFields((prev) => ({ ...prev, [key]: value }))
    setErrors((prev) => ({ ...prev, [key]: undefined }))
  }

  const validate = (): boolean => {
    const newErrors: LoginFormErrors = {}
    if (!fields.email.trim()) newErrors.email = 'Email is required'
    if (!fields.password) newErrors.password = 'Password is required'
    setErrors(newErrors)
    return Object.keys(newErrors).length === 0
  }

  const toRequest = (): LoginRequest => fields

  return { fields, errors, setField, validate, toRequest }
}
