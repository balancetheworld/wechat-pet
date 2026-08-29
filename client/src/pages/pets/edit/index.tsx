import { Button, Input, Text, View } from '@tarojs/components'
import Taro, { useRouter } from '@tarojs/taro'
import { useEffect, useState } from 'react'
import { createPet, getPet, updatePet } from '../../../services/pet'
import { navigateBack } from '../../../utils/navigation'

export default function PetEdit() {
  const { params } = useRouter<{ petId?: string }>()
  const petID = params.petId
  const [name, setName] = useState('')
  const [loading, setLoading] = useState(Boolean(petID))
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    if (!petID) {
      return
    }
    const id = petID
    async function load() {
      try {
        const pet = await getPet(id)
        setName(pet.name)
      }
      catch (error) {
        const message = error instanceof Error ? error.message : '加载宠物失败'
        await Taro.showToast({ title: message, icon: 'none' })
      }
      finally {
        setLoading(false)
      }
    }
    void load()
  }, [petID])

  async function handleSubmit() {
    const value = name.trim()
    if (value.length < 1 || value.length > 50) {
      await Taro.showToast({ title: '宠物名称长度需为 1-50 个字符', icon: 'none' })
      return
    }
    if (submitting || loading) {
      return
    }
    setSubmitting(true)
    try {
      if (petID) {
        await updatePet(petID, { name: value })
      }
      else {
        await createPet({ name: value })
      }
      await navigateBack()
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '保存失败，请重试'
      await Taro.showToast({ title: message, icon: 'none' })
    }
    finally {
      setSubmitting(false)
    }
  }

  return (
    <View>
      <Text>{petID ? '编辑宠物' : '创建宠物'}</Text>
      <Input
        maxlength={50}
        placeholder="请输入宠物名称"
        value={name}
        onInput={event => setName(event.detail.value)}
      />
      <Button loading={submitting || loading} disabled={submitting || loading} onClick={handleSubmit}>保存</Button>
    </View>
  )
}
